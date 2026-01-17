// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package gatewaylist

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=gatewaylists/finalizers,verbs=update

func (r *GatewayListReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the GatewayList instance
	list := &networkingv1alpha2.GatewayList{}
	if err := r.Get(ctx, req.NamespacedName, list); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve credentials
	credRef, accountID, err := r.resolveCredentials(ctx, list)
	if err != nil {
		logger.Error(err, "Failed to resolve credentials")
		return r.updateStatusError(ctx, list, err)
	}

	// Handle deletion
	if !list.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, list, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(list, FinalizerName) {
		controllerutil.AddFinalizer(list, FinalizerName)
		if err := r.Update(ctx, list); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register Gateway list configuration to SyncState
	return r.registerGatewayList(ctx, list, accountID, credRef)
}

// resolveCredentials resolves the credentials reference and account ID.
func (r *GatewayListReconciler) resolveCredentials(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
) (credRef networkingv1alpha2.CredentialsReference, accountID string, err error) {
	// Get credentials reference
	if list.Spec.Cloudflare.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: list.Spec.Cloudflare.CredentialsRef.Name,
		}

		// Get account ID from credentials if available
		creds := &networkingv1alpha2.CloudflareCredentials{}
		if err := r.Get(ctx, client.ObjectKey{Name: credRef.Name}, creds); err != nil {
			return credRef, "", fmt.Errorf("get credentials: %w", err)
		}
		accountID = creds.Spec.AccountID
	}

	if credRef.Name == "" {
		return credRef, "", errors.New("credentials reference is required")
	}

	return credRef, accountID, nil
}

func (r *GatewayListReconciler) handleDeletion(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	_ networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(list, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete from Cloudflare
	if result, shouldReturn := r.deleteFromCloudflare(ctx, list); shouldReturn {
		return result, nil
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "GatewayList",
		Namespace: "",
		Name:      list.Name,
	}
	if err := r.gatewayService.Unregister(ctx, list.Status.ListID, source); err != nil {
		logger.Error(err, "Failed to unregister Gateway list from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, list, func() {
		controllerutil.RemoveFinalizer(list, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// deleteFromCloudflare deletes the gateway list from Cloudflare.
// Returns (result, shouldReturn) where shouldReturn indicates if caller should return.
func (r *GatewayListReconciler) deleteFromCloudflare(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
) (ctrl.Result, bool) {
	if list.Status.ListID == "" {
		return ctrl.Result{}, false
	}

	logger := log.FromContext(ctx)
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, list.Spec.Cloudflare)
	if err != nil {
		logger.Error(err, "Failed to create API client for deletion")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, true
	}

	logger.Info("Deleting Gateway List from Cloudflare", "listId", list.Status.ListID)

	err = apiClient.DeleteGatewayList(list.Status.ListID)
	if err == nil {
		r.Recorder.Event(list, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	if cf.IsNotFoundError(err) {
		logger.Info("Gateway List already deleted from Cloudflare")
		r.Recorder.Event(list, corev1.EventTypeNormal, "AlreadyDeleted",
			"Gateway List was already deleted from Cloudflare")
		return ctrl.Result{}, false
	}

	logger.Error(err, "Failed to delete Gateway List from Cloudflare")
	r.Recorder.Event(list, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
		fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
	return ctrl.Result{RequeueAfter: 30 * time.Second}, true
}

// registerGatewayList registers the Gateway list configuration to SyncState.
func (r *GatewayListReconciler) registerGatewayList(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Collect items from spec and ConfigMap
	items, err := r.collectItems(ctx, list)
	if err != nil {
		r.Recorder.Event(list, corev1.EventTypeWarning, "CollectItemsFailed",
			fmt.Sprintf("Failed to collect items: %v", err))
		return r.updateStatusError(ctx, list, err)
	}

	// Build Gateway list configuration
	config := gatewaysvc.GatewayListConfig{
		Name:        list.GetGatewayListName(),
		Description: list.Spec.Description,
		Type:        list.Spec.Type,
		Items:       items,
	}

	// Create source reference
	source := service.Source{
		Kind:      "GatewayList",
		Namespace: "",
		Name:      list.Name,
	}

	// Register to SyncState
	opts := gatewaysvc.GatewayListRegisterOptions{
		AccountID:      accountID,
		ListID:         list.Status.ListID,
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.gatewayService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register Gateway list configuration")
		r.Recorder.Event(list, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register Gateway list: %s", err.Error()))
		return r.updateStatusError(ctx, list, err)
	}

	r.Recorder.Event(list, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered Gateway List '%s' configuration to SyncState", config.Name))

	// Update status to Pending - actual sync happens via GatewaySyncController
	return r.updateStatusPending(ctx, list, accountID, len(items))
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

func (r *GatewayListReconciler) updateStatusPending(
	ctx context.Context,
	list *networkingv1alpha2.GatewayList,
	accountID string,
	itemCount int,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, list, func() {
		if list.Status.AccountID == "" {
			list.Status.AccountID = accountID
		}
		list.Status.ItemCount = itemCount
		list.Status.State = "Pending"
		meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: list.Generation,
			Reason:             "Pending",
			Message:            "Gateway list configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		list.Status.ObservedGeneration = list.Generation
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
