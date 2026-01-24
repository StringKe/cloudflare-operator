// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package gatewaylist provides a controller for managing Cloudflare Gateway Lists.
// It directly calls Cloudflare API and writes status back to the CRD.
package gatewaylist

import (
	"context"
	"fmt"
	"strings"

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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
)

const (
	finalizerName = "gatewaylist.networking.cloudflare-operator.io/finalizer"
)

// Reconciler reconciles a GatewayList object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles GatewayList reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the GatewayList resource
	list := &networkingv1alpha2.GatewayList{}
	if err := r.Get(ctx, req.NamespacedName, list); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch GatewayList")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !list.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, list)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, list, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Get API client
	// GatewayList is cluster-scoped, use operator namespace for legacy inline secrets
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &list.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   list.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, list, err)
	}

	// Sync Gateway list to Cloudflare
	return r.syncGatewayList(ctx, list, apiResult)
}

// handleDeletion handles the deletion of GatewayList.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(list, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CloudflareDetails: &list.Spec.Cloudflare,
		Namespace:         common.OperatorNamespace,
		StatusAccountID:   list.Status.AccountID,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client for deletion")
		// Continue with finalizer removal
	} else if list.Status.ListID != "" {
		// Delete Gateway list from Cloudflare
		logger.Info("Deleting Gateway List from Cloudflare",
			"listId", list.Status.ListID)

		if err := apiResult.API.DeleteGatewayList(ctx, list.Status.ListID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete Gateway List from Cloudflare")
				r.Recorder.Event(list, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return common.RequeueShort(), err
			}
			logger.Info("Gateway List not found in Cloudflare, may have been already deleted")
		}

		r.Recorder.Event(list, corev1.EventTypeNormal, "Deleted",
			"Gateway List deleted from Cloudflare")
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, list, func() {
		controllerutil.RemoveFinalizer(list, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// syncGatewayList syncs the Gateway List to Cloudflare.
func (r *Reconciler) syncGatewayList(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine list name
	listName := list.GetGatewayListName()

	// Collect items from spec and ConfigMap
	items, err := r.collectItems(ctx, list)
	if err != nil {
		logger.Error(err, "Failed to collect items")
		r.Recorder.Event(list, corev1.EventTypeWarning, "CollectItemsFailed",
			fmt.Sprintf("Failed to collect items: %v", err))
		return r.updateStatusError(ctx, list, err)
	}

	// Build params
	params := cf.GatewayListParams{
		Name:        listName,
		Description: list.Spec.Description,
		Type:        list.Spec.Type,
		Items:       items,
	}

	// Check if list already exists by ID
	if list.Status.ListID != "" {
		existing, err := apiResult.API.GetGatewayList(ctx, list.Status.ListID)
		if err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to get Gateway List from Cloudflare")
				return r.updateStatusError(ctx, list, err)
			}
			// List doesn't exist, will create
			logger.Info("Gateway List not found in Cloudflare, will recreate",
				"listId", list.Status.ListID)
		} else {
			// List exists, update it
			logger.V(1).Info("Updating Gateway List in Cloudflare",
				"listId", existing.ID,
				"name", listName)

			result, err := apiResult.API.UpdateGatewayList(ctx, existing.ID, params)
			if err != nil {
				logger.Error(err, "Failed to update Gateway List")
				return r.updateStatusError(ctx, list, err)
			}

			r.Recorder.Event(list, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Gateway List '%s' updated in Cloudflare", listName))

			return r.updateStatusReady(ctx, list, apiResult.AccountID, result.ID, len(items))
		}
	}

	// Try to find existing list by name
	existingByName, err := apiResult.API.ListGatewayListsByName(ctx, listName)
	if err != nil && !cf.IsNotFoundError(err) {
		logger.Error(err, "Failed to search for existing Gateway List")
		return r.updateStatusError(ctx, list, err)
	}

	if existingByName != nil {
		// List already exists with this name, adopt it
		logger.Info("Gateway List already exists with same name, adopting it",
			"listId", existingByName.ID,
			"name", listName)

		// Update the existing list
		result, err := apiResult.API.UpdateGatewayList(ctx, existingByName.ID, params)
		if err != nil {
			logger.Error(err, "Failed to update existing Gateway List")
			return r.updateStatusError(ctx, list, err)
		}

		r.Recorder.Event(list, corev1.EventTypeNormal, "Adopted",
			fmt.Sprintf("Adopted existing Gateway List '%s'", listName))

		return r.updateStatusReady(ctx, list, apiResult.AccountID, result.ID, len(items))
	}

	// Create new list
	logger.Info("Creating Gateway List in Cloudflare",
		"name", listName)

	result, err := apiResult.API.CreateGatewayList(ctx, params)
	if err != nil {
		logger.Error(err, "Failed to create Gateway List")
		return r.updateStatusError(ctx, list, err)
	}

	r.Recorder.Event(list, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Gateway List '%s' created in Cloudflare", listName))

	return r.updateStatusReady(ctx, list, apiResult.AccountID, result.ID, len(items))
}

// collectItems collects items from spec and ConfigMap.
func (r *Reconciler) collectItems(ctx context.Context, list *networkingv1alpha2.GatewayList) ([]cf.GatewayListItem, error) {
	items := make([]cf.GatewayListItem, 0)

	// Add items from spec
	for _, item := range list.Spec.Items {
		items = append(items, cf.GatewayListItem{
			Value:       item.Value,
			Description: item.Description,
		})
	}

	// Add items from ConfigMap
	if list.Spec.ItemsFromConfigMap != nil {
		configMapItems, err := r.getItemsFromConfigMap(ctx, list)
		if err != nil {
			return nil, err
		}
		for _, value := range configMapItems {
			items = append(items, cf.GatewayListItem{
				Value: value,
			})
		}
	}

	return items, nil
}

// getItemsFromConfigMap gets items from the referenced ConfigMap.
func (r *Reconciler) getItemsFromConfigMap(ctx context.Context, list *networkingv1alpha2.GatewayList) ([]string, error) {
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

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, list, func() {
		list.Status.State = "Error"
		meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: list.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		list.Status.ObservedGeneration = list.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	accountID, listID string,
	itemCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, list, func() {
		list.Status.AccountID = accountID
		list.Status.ListID = listID
		list.Status.ItemCount = itemCount
		list.Status.State = "Ready"
		meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: list.Generation,
			Reason:             "Synced",
			Message:            "Gateway List synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		list.Status.ObservedGeneration = list.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	return common.NoRequeue(), nil
}

// gatewayListReferencesConfigMap checks if a GatewayList references the given ConfigMap.
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

// findGatewayListsForConfigMap returns reconcile requests for GatewayLists that reference the given ConfigMap.
func (r *Reconciler) findGatewayListsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
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
			logger.V(1).Info("ConfigMap changed, triggering GatewayList reconcile",
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gatewaylist-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("gatewaylist"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayList{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewayListsForConfigMap),
		).
		Named("gatewaylist").
		Complete(r)
}
