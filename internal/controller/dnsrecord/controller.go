// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package dnsrecord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	"github.com/StringKe/cloudflare-operator/internal/resolver"
)

const (
	FinalizerName = "dnsrecord.networking.cloudflare-operator.io/finalizer"
)

// DNSRecordReconciler reconciles a DNSRecord object
type DNSRecordReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	domainResolver *resolver.DomainResolver
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/finalizers,verbs=update

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

	// Initialize API client and resolve Zone ID
	apiClient, zoneID, err := r.initAPIClient(ctx, record)
	if err != nil {
		logger.Error(err, "Failed to initialize Cloudflare API client")
		return r.updateStatusError(ctx, record, err)
	}

	// Handle deletion
	if !record.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, record, apiClient, zoneID)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(record, FinalizerName) {
		controllerutil.AddFinalizer(record, FinalizerName)
		if err := r.Update(ctx, record); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the DNS record
	return r.reconcileDNSRecord(ctx, record, apiClient, zoneID)
}

func (r *DNSRecordReconciler) initAPIClient(ctx context.Context, record *networkingv1alpha2.DNSRecord) (*cf.API, string, error) {
	logger := log.FromContext(ctx)

	// Create API client
	apiClient, err := cf.NewAPIClientFromDetails(ctx, r.Client, record.Namespace, record.Spec.Cloudflare)
	if err != nil {
		return nil, "", err
	}

	// Determine Zone ID using priority:
	// 1. Explicit ZoneId in spec.cloudflare.zoneId
	// 2. Explicit Domain in spec.cloudflare.domain -> resolve via CloudflareDomain
	// 3. Use DomainResolver to find CloudflareDomain matching the record name
	// 4. Use existing apiClient.ValidZoneId (from domain lookup)
	zoneID := record.Spec.Cloudflare.ZoneId
	var resolvedDomain string

	if zoneID == "" && record.Spec.Cloudflare.Domain != "" {
		// Priority 2: Use explicit domain from spec to find CloudflareDomain
		domainInfo, err := r.domainResolver.Resolve(ctx, record.Spec.Cloudflare.Domain)
		if err != nil {
			logger.Error(err, "Failed to resolve explicit domain", "domain", record.Spec.Cloudflare.Domain)
		} else if domainInfo != nil {
			zoneID = domainInfo.ZoneID
			resolvedDomain = domainInfo.Domain
			logger.V(1).Info("Resolved Zone ID via explicit spec.cloudflare.domain",
				"name", record.Spec.Name,
				"specDomain", record.Spec.Cloudflare.Domain,
				"resolvedDomain", domainInfo.Domain,
				"zoneId", zoneID)
		}
	}

	if zoneID == "" {
		// Priority 3: Try DomainResolver using record name suffix matching
		domainInfo, err := r.domainResolver.Resolve(ctx, record.Spec.Name)
		if err != nil {
			logger.Error(err, "Failed to resolve domain for DNS record", "name", record.Spec.Name)
			// Fall back to existing Zone ID from API client
		} else if domainInfo != nil {
			zoneID = domainInfo.ZoneID
			resolvedDomain = domainInfo.Domain
			logger.V(1).Info("Resolved Zone ID via CloudflareDomain (name suffix match)",
				"name", record.Spec.Name,
				"domain", domainInfo.Domain,
				"zoneId", zoneID)
		}
	}

	// Priority 4: Use API client's ValidZoneId as final fallback
	if zoneID == "" {
		zoneID = apiClient.ValidZoneId
		resolvedDomain = apiClient.ValidDomainName
	}

	if zoneID == "" {
		return nil, "", fmt.Errorf("unable to determine Zone ID for DNS record %s: specify cloudflare.zoneId or create a CloudflareDomain resource", record.Spec.Name)
	}

	// Validate that the record name belongs to the resolved domain
	// This prevents creating records in the wrong zone (e.g., foo.sup-game.com in sup-any.com zone)
	if resolvedDomain != "" {
		if err := validateRecordBelongsToDomain(record.Spec.Name, resolvedDomain); err != nil {
			return nil, "", fmt.Errorf(
				"DNS record validation failed: %w. "+
					"Hint: create a CloudflareDomain resource for the correct domain or specify cloudflare.zoneId explicitly",
				err)
		}
	}

	return apiClient, zoneID, nil
}

func (r *DNSRecordReconciler) handleDeletion(ctx context.Context, record *networkingv1alpha2.DNSRecord, apiClient *cf.API, zoneID string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(record, FinalizerName) {
		// Delete from Cloudflare - use stored ZoneID from status or resolved zoneID
		effectiveZoneID := record.Status.ZoneID
		if effectiveZoneID == "" {
			effectiveZoneID = zoneID
		}

		if record.Status.RecordID != "" && effectiveZoneID != "" {
			logger.Info("Deleting DNS Record from Cloudflare", "recordId", record.Status.RecordID, "zoneId", effectiveZoneID)
			if err := apiClient.DeleteDNSRecordInZone(effectiveZoneID, record.Status.RecordID); err != nil {
				// P0 FIX: Check if resource already deleted
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete DNS Record from Cloudflare")
					r.Recorder.Event(record, corev1.EventTypeWarning, "DeleteFailed",
						fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				logger.Info("DNS Record already deleted from Cloudflare")
				r.Recorder.Event(record, corev1.EventTypeNormal, "AlreadyDeleted", "DNS Record was already deleted from Cloudflare")
			} else {
				r.Recorder.Event(record, corev1.EventTypeNormal, "Deleted", "Deleted from Cloudflare")
			}
		}

		// P0 FIX: Remove finalizer with retry logic to handle conflicts
		if err := controller.UpdateWithConflictRetry(ctx, r.Client, record, func() {
			controllerutil.RemoveFinalizer(record, FinalizerName)
		}); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(record, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) reconcileDNSRecord(ctx context.Context, record *networkingv1alpha2.DNSRecord, apiClient *cf.API, zoneID string) (ctrl.Result, error) {
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
		// Create new DNS record using InZone method
		logger.Info("Creating DNS Record", "name", params.Name, "type", params.Type, "zoneId", zoneID)
		r.Recorder.Event(record, corev1.EventTypeNormal, "Creating",
			fmt.Sprintf("Creating DNS Record '%s' (type: %s) in Cloudflare", params.Name, params.Type))
		result, err = apiClient.CreateDNSRecordInZone(zoneID, params)
		if err != nil {
			r.Recorder.Event(record, corev1.EventTypeWarning, controller.EventReasonCreateFailed,
				fmt.Sprintf("Failed to create DNS Record: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, record, err)
		}
		r.Recorder.Event(record, corev1.EventTypeNormal, controller.EventReasonCreated,
			fmt.Sprintf("Created DNS Record with ID '%s'", result.ID))
	} else {
		// Update existing DNS record using InZone method
		// Use stored ZoneID from status if available, otherwise use resolved zoneID
		effectiveZoneID := record.Status.ZoneID
		if effectiveZoneID == "" {
			effectiveZoneID = zoneID
		}

		logger.Info("Updating DNS Record", "recordId", record.Status.RecordID, "zoneId", effectiveZoneID)
		r.Recorder.Event(record, corev1.EventTypeNormal, "Updating",
			fmt.Sprintf("Updating DNS Record '%s' in Cloudflare", record.Status.RecordID))
		result, err = apiClient.UpdateDNSRecordInZone(effectiveZoneID, record.Status.RecordID, params)
		if err != nil {
			r.Recorder.Event(record, corev1.EventTypeWarning, controller.EventReasonUpdateFailed,
				fmt.Sprintf("Failed to update DNS Record: %s", cf.SanitizeErrorMessage(err)))
			return r.updateStatusError(ctx, record, err)
		}
		r.Recorder.Event(record, corev1.EventTypeNormal, controller.EventReasonUpdated,
			fmt.Sprintf("Updated DNS Record '%s'", result.ID))
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
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, record, func() {
		record.Status.State = "Error"
		meta.SetStatusCondition(&record.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: record.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		record.Status.ObservedGeneration = record.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DNSRecordReconciler) updateStatusSuccess(ctx context.Context, record *networkingv1alpha2.DNSRecord, result *cf.DNSRecordResult) (ctrl.Result, error) {
	// P0 FIX: Use UpdateStatusWithConflictRetry for status updates
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, record, func() {
		record.Status.RecordID = result.ID
		record.Status.ZoneID = result.ZoneID
		record.Status.FQDN = result.Name
		record.Status.State = "Ready"
		meta.SetStatusCondition(&record.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: record.Generation,
			Reason:             controller.EventReasonReconciled,
			Message:            "DNS Record successfully reconciled",
			LastTransitionTime: metav1.Now(),
		})
		record.Status.ObservedGeneration = record.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// findDNSRecordsForDomain returns DNSRecords that may need reconciliation when a CloudflareDomain changes
func (r *DNSRecordReconciler) findDNSRecordsForDomain(ctx context.Context, obj client.Object) []reconcile.Request {
	domain, ok := obj.(*networkingv1alpha2.CloudflareDomain)
	if !ok {
		return nil
	}

	// List all DNSRecords
	recordList := &networkingv1alpha2.DNSRecordList{}
	if err := r.List(ctx, recordList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, record := range recordList.Items {
		// Check if this record's name is a suffix of the domain or equals the domain
		// This is a simple heuristic - records ending with ".domain" or equal to "domain" may be affected
		if record.Spec.Name == domain.Spec.Domain ||
			len(record.Spec.Name) > len(domain.Spec.Domain) &&
				record.Spec.Name[len(record.Spec.Name)-len(domain.Spec.Domain)-1:] == "."+domain.Spec.Domain {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&record),
			})
		}
	}

	return requests
}

// validateRecordBelongsToDomain checks if a DNS record name belongs to a domain.
// For example:
//   - "api.example.com" belongs to "example.com" ✓
//   - "api.staging.example.com" belongs to "example.com" ✓
//   - "example.com" belongs to "example.com" ✓
//   - "api.other.com" does NOT belong to "example.com" ✗
//   - "notexample.com" does NOT belong to "example.com" ✗
func validateRecordBelongsToDomain(recordName, domainName string) error {
	// Exact match
	if recordName == domainName {
		return nil
	}

	// Suffix match: recordName must end with ".domainName"
	suffix := "." + domainName
	if strings.HasSuffix(recordName, suffix) {
		return nil
	}

	return fmt.Errorf("record name %q does not belong to domain %q (expected %q or *%s)",
		recordName, domainName, domainName, suffix)
}

func (r *DNSRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("dnsrecord-controller")

	// Initialize DomainResolver
	r.domainResolver = resolver.NewDomainResolver(mgr.GetClient(), logr.Discard())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DNSRecord{}).
		Watches(&networkingv1alpha2.CloudflareDomain{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForDomain)).
		Complete(r)
}
