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

package gatewaylist

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	FinalizerName = "gatewaylist.networking.cloudflare-operator.io/finalizer"
)

// GatewayListReconciler reconciles a GatewayList object
type GatewayListReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/finalizers,verbs=update

func (r *GatewayListReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayList instance
	list := &networkingv1alpha2.GatewayList{}
	if err := r.Get(ctx, req.NamespacedName, list); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	// GatewayList is cluster-scoped, use operator namespace for legacy inline secrets
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, list.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, list, err)
	}

	// Handle deletion
	if !list.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, list, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(list, FinalizerName) {
		controllerutil.AddFinalizer(list, FinalizerName)
		if err := r.Update(ctx, list); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the gateway list
	return r.reconcileGatewayList(ctx, list, apiClient)
}

func (r *GatewayListReconciler) handleDeletion(ctx context.Context, list *networkingv1alpha2.GatewayList, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(list, FinalizerName) {
		// Delete from Cloudflare
		if list.Status.ListID != "" {
			logger.Info("Deleting Gateway List from Cloudflare", "listId", list.Status.ListID)
			if err := apiClient.DeleteGatewayList(list.Status.ListID); err != nil {
				// P0 FIX: Check if resource already deleted
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete Gateway List from Cloudflare")
					r.Recorder.Event(list, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("Gateway List already deleted from Cloudflare")
				r.Recorder.Event(list, corev1.EventTypeNormal, "AlreadyDeleted", "Gateway List was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
			}
		}

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, list, func() {
			controllerutil.RemoveFinalizer(list, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *GatewayListReconciler) reconcileGatewayList(ctx context.Context, list *networkingv1alpha2.GatewayList, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Collect items from spec and ConfigMap
	items, err := r.collectItems(ctx, list)
	if err != nil {
		r.Recorder.Event(list, corev1.EventTypeWarning, "CollectItemsFailed",
			fmt.Sprintf("Failed to collect items: %v", err))
		return r.updateStatusError(ctx, list, err)
	}

	// Build gateway list params
	params := cf.GatewayListParams{
		Name:        list.GetGatewayListName(),
		Description: list.Spec.Description,
		Type:        list.Spec.Type,
		Items:       items,
	}

	var result *cf.GatewayListResult

	if list.Status.ListID == "" {
		// Create new gateway list
		logger.Info("Creating Gateway List", "name", params.Name, "type", params.Type)
		r.Recorder.Event(list, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating Gateway List '%s' (type: %s, items: %d) in Cloudflare", params.Name, params.Type, len(items)))
		result, err = apiClient.CreateGatewayList(params)
		if err != nil {
			r.Recorder.Event(list, corev1.EventTypeWarning, controller.EventReasonCreateFailed,
				fmt.Sprintf("Failed to create Gateway List: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, list, err)
		}
		r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonCreated,
			fmt.Sprintf("Created Gateway List with ID '%s'", result.ID))
	} else {
		// Update existing gateway list
		logger.Info("Updating Gateway List", "listId", list.Status.ListID)
		r.Recorder.Event(list, corev1.EventTypeNormal, "Updating",
			fmt.Sprintf("Updating Gateway List '%s' (items: %d) in Cloudflare", list.Status.ListID, len(items)))
		result, err = apiClient.UpdateGatewayList(list.Status.ListID, params)
		if err != nil {
			r.Recorder.Event(list, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
				fmt.Sprintf("Failed to update Gateway List: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, list, err)
		}
		r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonUpdated,
			fmt.Sprintf("Updated Gateway List '%s'", result.ID))
	}

	// Update status
	return r.updateStatusSuccess(ctx, list, result, len(items))
}

func (r *GatewayListReconciler) collectItems(ctx context.Context, list *networkingv1alpha2.GatewayList) ([]string, error) {
	items := make([]string, 0)

	// Add items from spec
	for _, item := range list.Spec.Items {
		items = append(items, item.Value)
	}

	// Add items from ConfigMap
	if list.Spec.ItemsFromConfigMap != nil {
		configMapItems, err := r.getItemsFromConfigMap(ctx, list)
		if err != nil {
			return nil, err
		}
		items = append(items, configMapItems...)
	}

	return items, nil
}

func (r *GatewayListReconciler) getItemsFromConfigMap(ctx context.Context, list *networkingv1alpha2.GatewayList) ([]string, error) {
	ref := list.Spec.ItemsFromConfigMap

	namespace := ref.Namespace
	if namespace == "" {
		namespace = "default"
	}

	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, configMap); err != nil {
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

func (r *GatewayListReconciler) updateStatusError(ctx context.Context, list *networkingv1alpha2.GatewayList, err error) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, list, func() {
		list.Status.State = "Error"
		meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: list.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		list.Status.ObservedGeneration = list.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayListReconciler) updateStatusSuccess(ctx context.Context, list *networkingv1alpha2.GatewayList, result *cf.GatewayListResult, itemCount int) (ctrl.Result, error) {
	// Use retry logic for status updates to handle conflicts
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, list, func() {
		list.Status.ListID = result.ID
		list.Status.AccountID = result.AccountID
		list.Status.ItemCount = itemCount
		list.Status.State = "Ready"
		meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: list.Generation,
			Reason:             "Reconciled",
			Message:            "Gateway List successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		list.Status.ObservedGeneration = list.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
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
	logger := log.FromContext(ctx)

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
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayList{}).
		// P2 FIX: Watch ConfigMap changes to trigger GatewayList reconcile
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewayListsForConfigMap),
		).
		Complete(r)
}
