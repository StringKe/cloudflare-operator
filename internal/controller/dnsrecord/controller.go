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

package dnsrecord

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
)

const (
	FinalizerName = "dnsrecord.networking.cfargotunnel.com/finalizer"
)

// DNSRecordReconciler reconciles a DNSRecord object
type DNSRecordReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cfargotunnel.com,resources=dnsrecords/finalizers,verbs=update

func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DNSRecord instance
	record := &networkingv1alpha2.DNSRecord{}
	if err := r.Get(ctx, req.NamespacedName, record); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize API client
	apiClient, err := r.initAPIClient(ctx, record)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, record, err)
	}

	// Handle deletion
	if !record.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, record, apiClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(record, FinalizerName) {
		controllerutil.AddFinalizer(record, FinalizerName)
		if err := r.Update(ctx, record); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the DNS record
	return r.reconcileDNSRecord(ctx, record, apiClient)
}

func (r *DNSRecordReconciler) initAPIClient(ctx context.Context, record *networkingv1alpha2.DNSRecord) (*cf.API, error) {
	return cf.NewAPIClientFromDetails(ctx, r.Client, record.Namespace, record.Spec.Cloudflare)
}

func (r *DNSRecordReconciler) handleDeletion(ctx context.Context, record *networkingv1alpha2.DNSRecord, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(record, FinalizerName) {
		// Delete from Cloudflare
		if record.Status.RecordID != "" && record.Status.ZoneID != "" {
			logger.Info("Deleting DNS Record from Cloudflare", "recordId", record.Status.RecordID)
			if err := apiClient.DeleteDNSRecord(record.Status.ZoneID, record.Status.RecordID); err != nil {
				logger.Error(err, "Failed to delete DNS Record from Cloudflare")
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(record, FinalizerName)
		if err := r.Update(ctx, record); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) reconcileDNSRecord(ctx context.Context, record *networkingv1alpha2.DNSRecord, apiClient *cf.API) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build DNS record params
	params := cf.DNSRecordParams{
		Name:    record.Spec.Name,
		Type:    record.Spec.Type,
		Content: record.Spec.Content,
		TTL:     record.Spec.TTL,
		Proxied: record.Spec.Proxied,
		Comment: record.Spec.Comment,
		Tags:    record.Spec.Tags,
	}

	if record.Spec.Priority != nil {
		params.Priority = record.Spec.Priority
	}

	// Build data for special record types
	if record.Spec.Data != nil {
		params.Data = r.buildRecordData(record.Spec.Data)
	}

	var result *cf.DNSRecordResult
	var err error

	if record.Status.RecordID == "" {
		// Create new DNS record
		logger.Info("Creating DNS Record", "name", params.Name, "type", params.Type)
		result, err = apiClient.CreateDNSRecord(params)
	} else {
		// Update existing DNS record
		logger.Info("Updating DNS Record", "recordId", record.Status.RecordID)
		result, err = apiClient.UpdateDNSRecord(record.Status.ZoneID, record.Status.RecordID, params)
	}

	if err != nil {
		return r.updateStatusError(ctx, record, err)
	}

	// Update status
	return r.updateStatusSuccess(ctx, record, result)
}

func (r *DNSRecordReconciler) buildRecordData(data *networkingv1alpha2.DNSRecordData) map[string]interface{} {
	result := make(map[string]interface{})

	// SRV record data
	if data.Service != "" {
		result["service"] = data.Service
	}
	if data.Proto != "" {
		result["proto"] = data.Proto
	}
	if data.Weight != 0 {
		result["weight"] = data.Weight
	}
	if data.Port != 0 {
		result["port"] = data.Port
	}
	if data.Target != "" {
		result["target"] = data.Target
	}

	// CAA record data
	if data.Flags != 0 {
		result["flags"] = data.Flags
	}
	if data.Tag != "" {
		result["tag"] = data.Tag
	}
	if data.Value != "" {
		result["value"] = data.Value
	}

	// CERT/SSHFP/TLSA record data
	if data.Algorithm != 0 {
		result["algorithm"] = data.Algorithm
	}
	if data.Certificate != "" {
		result["certificate"] = data.Certificate
	}
	if data.KeyTag != 0 {
		result["key_tag"] = data.KeyTag
	}
	if data.Usage != 0 {
		result["usage"] = data.Usage
	}
	if data.Selector != 0 {
		result["selector"] = data.Selector
	}
	if data.MatchingType != 0 {
		result["matching_type"] = data.MatchingType
	}

	// LOC record data
	if data.LatDegrees != 0 {
		result["lat_degrees"] = data.LatDegrees
	}
	if data.LatMinutes != 0 {
		result["lat_minutes"] = data.LatMinutes
	}
	if data.LatSeconds != "" {
		result["lat_seconds"] = data.LatSeconds
	}
	if data.LatDirection != "" {
		result["lat_direction"] = data.LatDirection
	}
	if data.LongDegrees != 0 {
		result["long_degrees"] = data.LongDegrees
	}
	if data.LongMinutes != 0 {
		result["long_minutes"] = data.LongMinutes
	}
	if data.LongSeconds != "" {
		result["long_seconds"] = data.LongSeconds
	}
	if data.LongDirection != "" {
		result["long_direction"] = data.LongDirection
	}
	if data.Altitude != "" {
		result["altitude"] = data.Altitude
	}
	if data.Size != "" {
		result["size"] = data.Size
	}
	if data.PrecisionHorz != "" {
		result["precision_horz"] = data.PrecisionHorz
	}
	if data.PrecisionVert != "" {
		result["precision_vert"] = data.PrecisionVert
	}

	// URI record data
	if data.ContentURI != "" {
		result["content"] = data.ContentURI
	}

	return result
}

func (r *DNSRecordReconciler) updateStatusError(ctx context.Context, record *networkingv1alpha2.DNSRecord, err error) (ctrl.Result, error) {
	record.Status.State = "Error"
	meta.SetStatusCondition(&record.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})
	record.Status.ObservedGeneration = record.Generation

	if updateErr := r.Status().Update(ctx, record); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DNSRecordReconciler) updateStatusSuccess(ctx context.Context, record *networkingv1alpha2.DNSRecord, result *cf.DNSRecordResult) (ctrl.Result, error) {
	record.Status.RecordID = result.ID
	record.Status.ZoneID = result.ZoneID
	record.Status.FQDN = result.Name
	record.Status.State = "Ready"
	meta.SetStatusCondition(&record.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "DNS Record successfully reconciled",
		LastTransitionTime: metav1.Now(),
	})
	record.Status.ObservedGeneration = record.Generation

	if err := r.Status().Update(ctx, record); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *DNSRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DNSRecord{}).
		Complete(r)
}
