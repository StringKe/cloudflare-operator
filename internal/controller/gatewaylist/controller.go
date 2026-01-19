// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewaylist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	gatewaysvc "github.com/StringKe/cloudflare-operator/internal/service/gateway"
)

const (
	FinalizerName = "gatewaylist.networking.cloudflare-operator.io/finalizer"
)

// GatewayListReconciler reconciles a GatewayList object
type GatewayListReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	gatewayService *gatewaysvc.GatewayListService

	// Runtime state
	ctx  context.Context
	log  logr.Logger
	list *networkingv1alpha2.GatewayList
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/finalizers,verbs=update

func (r *GatewayListReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the GatewayList instance
	r.list = &networkingv1alpha2.GatewayList{}
	if err := r.Get(ctx, req.NamespacedName, r.list); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credInfo, err := r.resolveCredentials()
	if err != nil {
		r.log.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(err)
	}

	// Handle deletion
	if !r.list.DeletionTimestamp.IsZero() {
		return r.handleDeletion(credInfo)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.list, FinalizerName) {
		controllerutil.AddFinalizer(r.list, FinalizerName)
		if err := r.Update(ctx, r.list); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Gateway list configuration to SyncState
	return r.registerGatewayList(credInfo)
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *GatewayListReconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// GatewayList is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.list.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.list.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.list, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of a GatewayList.
// Following Unified Sync Architecture: Resource Controller only unregisters from SyncState,
// the L5 Sync Controller handles actual Cloudflare API deletion.
func (r *GatewayListReconciler) handleDeletion(
	_ *controller.CredentialsInfo,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.list, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Unregister from SyncState - L5 Sync Controller will handle Cloudflare deletion
	source := service.Source{
		Kind:      "GatewayList",
		Namespace: "",
		Name:      r.list.Name,
	}
	if err := r.gatewayService.Unregister(r.ctx, r.list.Status.ListID, source); err != nil {
		r.log.Error(err, "Failed to unregister Gateway list from SyncState")
		r.Recorder.Event(r.list, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.log.Info("Unregistered Gateway list from SyncState, L5 Sync Controller will handle Cloudflare deletion")
	r.Recorder.Event(r.list, corev1.EventTypeNormal, "Unregistered", "Unregistered from SyncState")

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.list, func() {
		controllerutil.RemoveFinalizer(r.list, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.list, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerGatewayList registers the Gateway list configuration to SyncState.
func (r *GatewayListReconciler) registerGatewayList(
	credInfo *controller.CredentialsInfo,
) (ctrl.Result, error) {
	// Collect items from spec and ConfigMap
	items, err := r.collectItems()
	if err != nil {
		r.Recorder.Event(r.list, corev1.EventTypeWarning, "CollectItemsFailed",
			fmt.Sprintf("Failed to collect items: %v", err))
		return r.updateStatusError(err)
	}

	// Build Gateway list configuration
	config := gatewaysvc.GatewayListConfig{
		Name:        r.list.GetGatewayListName(),
		Description: r.list.Spec.Description,
		Type:        r.list.Spec.Type,
		Items:       items,
	}

	// Create source reference
	source := service.Source{
		Kind:      "GatewayList",
		Namespace: "",
		Name:      r.list.Name,
	}

	// Register to SyncState
	opts := gatewaysvc.GatewayListRegisterOptions{
		AccountID:      credInfo.AccountID,
		ListID:         r.list.Status.ListID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.gatewayService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "Failed to register Gateway list configuration")
		r.Recorder.Event(r.list, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Gateway list: %s", err.Error()))
		return r.updateStatusError(err)
	}

	r.Recorder.Event(r.list, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Gateway List '%s' configuration to SyncState", config.Name))

	// Update status to Pending - actual sync happens via GatewaySyncController
	return r.updateStatusPending(credInfo.AccountID, len(items))
}

func (r *GatewayListReconciler) collectItems() ([]gatewaysvc.GatewayListItem, error) {
	items := make([]gatewaysvc.GatewayListItem, 0)

	// Add items from spec with descriptions
	for _, item := range r.list.Spec.Items {
		items = append(items, gatewaysvc.GatewayListItem{
			Value:       item.Value,
			Description: item.Description,
		})
	}

	// Add items from ConfigMap (ConfigMap items don't have descriptions)
	if r.list.Spec.ItemsFromConfigMap != nil {
		configMapItems, err := r.getItemsFromConfigMap()
		if err != nil {
			return nil, err
		}
		for _, value := range configMapItems {
			items = append(items, gatewaysvc.GatewayListItem{
				Value: value,
			})
		}
	}

	return items, nil
}

func (r *GatewayListReconciler) getItemsFromConfigMap() ([]string, error) {
	ref := r.list.Spec.ItemsFromConfigMap

	namespace := ref.Namespace
	if namespace == "" {
		namespace = "default"
	}

	configMap := &corev1.ConfigMap{}
	if err := r.Get(r.ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, configMap); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, ref.Name, err)
	}

	data, ok := configMap.Data[ref.Key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in ConfigMap %s/%s", ref.Key, namespace, ref.Name)
	}

	// Parse items (one per line)
	items := make([]string, 0)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		items = append(items, line)
	}

	return items, nil
}

func (r *GatewayListReconciler) updateStatusError(err error) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.list, func() {
		r.list.Status.State = "Error"
		meta.SetStatusCondition(&r.list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.list.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		r.list.Status.ObservedGeneration = r.list.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayListReconciler) updateStatusPending(
	accountID string,
	itemCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.list, func() {
		if r.list.Status.AccountID == "" {
			r.list.Status.AccountID = accountID
		}
		r.list.Status.ItemCount = itemCount
		r.list.Status.State = "Pending"
		meta.SetStatusCondition(&r.list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: r.list.Generation,
			Reason:             "Pending",
			Message:            "Gateway list configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		r.list.Status.ObservedGeneration = r.list.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// gatewayListReferencesConfigMap checks if a GatewayList references the given ConfigMap
func gatewayListReferencesConfigMap(gl *networkingv1alpha2.GatewayList, cmName, cmNamespace string) bool {
	if gl.Spec.ItemsFromConfigMap == nil {
		return false
	}
	refNamespace := gl.Spec.ItemsFromConfigMap.Namespace
	if refNamespace == "" {
		refNamespace = "default"
	}
	return gl.Spec.ItemsFromConfigMap.Name == cmName && refNamespace == cmNamespace
}

// findGatewayListsForConfigMap returns a list of reconcile requests for GatewayLists
// that reference the given ConfigMap.
func (r *GatewayListReconciler) findGatewayListsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	gatewayLists := &networkingv1alpha2.GatewayListList{}
	if err := r.List(ctx, gatewayLists); err != nil {
		logger.Error(err, "Failed to list GatewayLists for ConfigMap watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range gatewayLists.Items {
		gl := &gatewayLists.Items[i]
		if gatewayListReferencesConfigMap(gl, configMap.Name, configMap.Namespace) {
			logger.Info("ConfigMap changed, triggering GatewayList reconcile",
				"configmap", configMap.Name,
				"configmapNamespace", configMap.Namespace,
				"gatewaylist", gl.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: gl.Name},
			})
		}
	}

	return requests
}

func (r *GatewayListReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewaylist-controller")

	// Initialize GatewayListService
	r.gatewayService = gatewaysvc.NewGatewayListService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayList{}).
		// Watch ConfigMap changes to trigger GatewayList reconcile
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewayListsForConfigMap),
		).
		Complete(r)
}
