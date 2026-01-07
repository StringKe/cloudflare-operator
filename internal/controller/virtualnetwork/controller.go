/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package virtualnetwork

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudflare/cloudflare-go"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

// Reconciler reconciles a VirtualNetwork object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Runtime state
	ctx   context.Context
	log   logr.Logger
	vnet  *networkingv1alpha2.VirtualNetwork
	cfAPI *cf.API
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=virtualnetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for VirtualNetwork resources.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the VirtualNetwork resource
	r.vnet = &networkingv1alpha2.VirtualNetwork{}
	if err := r.Get(ctx, req.NamespacedName, r.vnet); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("VirtualNetwork deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch VirtualNetwork")
		return ctrl.Result{}, err
	}

	// Initialize API client
	if err := r.initAPIClient(); err != nil {
		r.log.Error(err, "failed to initialize API client")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	// Check if VirtualNetwork is being deleted
	if r.vnet.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.vnet, controller.VirtualNetworkFinalizer) {
		controllerutil.AddFinalizer(r.vnet, controller.VirtualNetworkFinalizer)
		if err := r.Update(ctx, r.vnet); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the VirtualNetwork
	if err := r.reconcileVirtualNetwork(); err != nil {
		r.log.Error(err, "failed to reconcile VirtualNetwork")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// initAPIClient initializes the Cloudflare API client from the referenced Secret.
func (r *Reconciler) initAPIClient() error {
	// Get the secret containing API credentials
	secret := &corev1.Secret{}
	secretName := r.vnet.Spec.Cloudflare.Secret
	// VirtualNetwork is cluster-scoped, so we need to determine the namespace for the secret
	// Use the namespace from the secret reference if provided in the spec
	// For now, we'll require the secret to be in a well-known namespace or use a ConfigMap to specify it
	// We'll use the default namespace for cluster-scoped resources based on the controller config

	// Try to get secret from the cloudflare spec - for cluster-scoped resources,
	// we need to determine namespace. Let's check if there's an accountId or use default namespace.
	namespace := "cloudflare-operator-system" // default namespace for cluster resources

	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		r.log.Error(err, "failed to get secret", "secret", secretName, "namespace", namespace)
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonAPIError, "Failed to get API secret")
		return err
	}

	// Extract API token or key
	apiToken := string(secret.Data[r.vnet.Spec.Cloudflare.CLOUDFLARE_API_TOKEN])
	apiKey := string(secret.Data[r.vnet.Spec.Cloudflare.CLOUDFLARE_API_KEY])

	if apiToken == "" && apiKey == "" {
		err := fmt.Errorf("neither API token nor API key found in secret")
		r.log.Error(err, "missing credentials in secret")
		return err
	}

	// Create cloudflare client
	var cloudflareClient *cloudflare.API
	var err error
	if apiToken != "" {
		cloudflareClient, err = cloudflare.NewWithAPIToken(apiToken)
	} else {
		cloudflareClient, err = cloudflare.New(apiKey, r.vnet.Spec.Cloudflare.Email)
	}
	if err != nil {
		r.log.Error(err, "failed to create cloudflare client")
		return err
	}

	r.cfAPI = &cf.API{
		Log:              r.log,
		AccountName:      r.vnet.Spec.Cloudflare.AccountName,
		AccountId:        r.vnet.Spec.Cloudflare.AccountId,
		ValidAccountId:   r.vnet.Status.AccountId,
		CloudflareClient: cloudflareClient,
	}

	return nil
}

// handleDeletion handles the deletion of a VirtualNetwork.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.vnet, controller.VirtualNetworkFinalizer) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Deleting VirtualNetwork")
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Deleting", "Starting VirtualNetwork deletion")

	// Delete from Cloudflare if it exists
	if r.vnet.Status.VirtualNetworkId != "" {
		if err := r.cfAPI.DeleteVirtualNetwork(r.vnet.Status.VirtualNetworkId); err != nil {
			r.log.Error(err, "failed to delete VirtualNetwork from Cloudflare")
			r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonDeleteFailed, err.Error())
			return ctrl.Result{}, err
		}
		r.log.Info("VirtualNetwork deleted from Cloudflare", "id", r.vnet.Status.VirtualNetworkId)
		r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(r.vnet, controller.VirtualNetworkFinalizer)
	if err := r.Update(r.ctx, r.vnet); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileVirtualNetwork ensures the VirtualNetwork exists in Cloudflare with the correct configuration.
func (r *Reconciler) reconcileVirtualNetwork() error {
	vnetName := r.vnet.GetVirtualNetworkName()

	// Check if VirtualNetwork already exists in Cloudflare
	if r.vnet.Status.VirtualNetworkId != "" {
		// Update existing VirtualNetwork
		return r.updateVirtualNetwork(vnetName)
	}

	// Try to find existing VirtualNetwork by name
	existing, err := r.cfAPI.GetVirtualNetworkByName(vnetName)
	if err == nil && existing != nil {
		// Found existing - adopt it
		r.log.Info("Found existing VirtualNetwork, adopting", "id", existing.ID, "name", existing.Name)
		return r.updateStatus(existing)
	}

	// Create new VirtualNetwork
	return r.createVirtualNetwork(vnetName)
}

// createVirtualNetwork creates a new VirtualNetwork in Cloudflare.
func (r *Reconciler) createVirtualNetwork(name string) error {
	r.log.Info("Creating VirtualNetwork", "name", name)
	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, "Creating", fmt.Sprintf("Creating VirtualNetwork: %s", name))

	result, err := r.cfAPI.CreateVirtualNetwork(cf.VirtualNetworkParams{
		Name:             name,
		Comment:          r.vnet.Spec.Comment,
		IsDefaultNetwork: r.vnet.Spec.IsDefaultNetwork,
	})
	if err != nil {
		r.log.Error(err, "failed to create VirtualNetwork")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonCreated, fmt.Sprintf("Created VirtualNetwork: %s", result.ID))
	return r.updateStatus(result)
}

// updateVirtualNetwork updates an existing VirtualNetwork in Cloudflare.
func (r *Reconciler) updateVirtualNetwork(name string) error {
	r.log.Info("Updating VirtualNetwork", "id", r.vnet.Status.VirtualNetworkId, "name", name)

	result, err := r.cfAPI.UpdateVirtualNetwork(r.vnet.Status.VirtualNetworkId, cf.VirtualNetworkParams{
		Name:             name,
		Comment:          r.vnet.Spec.Comment,
		IsDefaultNetwork: r.vnet.Spec.IsDefaultNetwork,
	})
	if err != nil {
		r.log.Error(err, "failed to update VirtualNetwork")
		r.Recorder.Event(r.vnet, corev1.EventTypeWarning, controller.EventReasonUpdateFailed, err.Error())
		r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.vnet, corev1.EventTypeNormal, controller.EventReasonUpdated, "VirtualNetwork updated")
	return r.updateStatus(result)
}

// updateStatus updates the VirtualNetwork status with the Cloudflare state.
func (r *Reconciler) updateStatus(result *cf.VirtualNetworkResult) error {
	r.vnet.Status.VirtualNetworkId = result.ID
	r.vnet.Status.AccountId = r.cfAPI.ValidAccountId
	r.vnet.Status.IsDefault = result.IsDefaultNetwork
	r.vnet.Status.ObservedGeneration = r.vnet.Generation

	if result.DeletedAt != nil {
		r.vnet.Status.State = "deleted"
	} else {
		r.vnet.Status.State = "active"
	}

	r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "VirtualNetwork reconciled successfully")

	if err := r.Status().Update(r.ctx, r.vnet); err != nil {
		r.log.Error(err, "failed to update VirtualNetwork status")
		return err
	}

	r.log.Info("VirtualNetwork status updated", "id", r.vnet.Status.VirtualNetworkId, "state", r.vnet.Status.State)
	return nil
}

// setCondition sets a condition on the VirtualNetwork status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.vnet.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Update or append condition
	found := false
	for i, c := range r.vnet.Status.Conditions {
		if c.Type == condition.Type {
			r.vnet.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.vnet.Status.Conditions = append(r.vnet.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("virtualnetwork-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.VirtualNetwork{}).
		Complete(r)
}
