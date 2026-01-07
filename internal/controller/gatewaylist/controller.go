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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "gatewaylist.networking.cloudflare-operator.io/finalizer"
)

// GatewayListReconciler reconciles a GatewayList object
type GatewayListReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, "", list.Spec.Cloudflare)
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
				logger.Error(err, "Failed to delete Gateway List from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(list, FinalizerName)
		if err := r.Update(ctx, list); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *GatewayListReconciler) reconcileGatewayList(ctx context.Context, list *networkingv1alpha2.GatewayList, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Collect items from spec and ConfigMap
	items, err := r.collectItems(ctx, list)
	if err != nil {
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
		result, err = apiClient.CreateGatewayList(params)
	} else {
		// Update existing gateway list
		logger.Info("Updating Gateway List", "listId", list.Status.ListID)
		result, err = apiClient.UpdateGatewayList(list.Status.ListID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, list, err)
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
	list.Status.State = "Error"
	meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	list.Status.ObservedGeneration = list.Generation

	if updateErr := r.Status().Update(ctx, list); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *GatewayListReconciler) updateStatusSuccess(ctx context.Context, list *networkingv1alpha2.GatewayList, result *cf.GatewayListResult, itemCount int) (ctrl.Result, error) {
	list.Status.ListID = result.ID
	list.Status.AccountID = result.AccountID
	list.Status.ItemCount = itemCount
	list.Status.State = "Ready"
	meta.SetStatusCondition(&list.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Gateway List successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	list.Status.ObservedGeneration = list.Generation

	if err := r.Status().Update(ctx, list); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GatewayListReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.GatewayList{}).
		Complete(r)
}
