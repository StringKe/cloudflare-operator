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
	"github.com/StringKe/cloudflare-operator/internal/service"
	dnssvc "github.com/StringKe/cloudflare-operator/internal/service/dns"
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
	dnsService     *dnssvc.Service
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

	// Resolve Zone ID and credentials
	zoneID, accountID, credRef, err := r.resolveZoneAndCredentials(ctx, record)
	if err != nil {
		logger.Error(err, "Failed to resolve zone and credentials")
		return r.updateStatusError(ctx, record, err)
	}

	// Handle deletion
	if !record.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, record, zoneID, credRef)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(record, FinalizerName) {
		controllerutil.AddFinalizer(record, FinalizerName)
		if err := r.Update(ctx, record); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Register DNS record configuration to SyncState
	return r.registerDNSRecord(ctx, record, zoneID, accountID, credRef)
}

// resolveZoneAndCredentials resolves the Zone ID, Account ID, and credentials reference.
//
//nolint:revive // cognitive complexity is acceptable for this resolution function
func (r *DNSRecordReconciler) resolveZoneAndCredentials(
	ctx context.Context,
	record *networkingv1alpha2.DNSRecord,
) (zoneID, accountID string, credRef networkingv1alpha2.CredentialsReference, err error) {
	logger := log.FromContext(ctx)

	// Get credentials reference
	if record.Spec.Cloudflare.CredentialsRef != nil {
		credRef = networkingv1alpha2.CredentialsReference{
			Name: record.Spec.Cloudflare.CredentialsRef.Name,
		}
	}

	// Determine Zone ID using priority:
	// 1. Explicit ZoneId in spec.cloudflare.zoneId
	// 2. Explicit Domain in spec.cloudflare.domain -> resolve via CloudflareDomain
	// 3. Use DomainResolver to find CloudflareDomain matching the record name
	zoneID = record.Spec.Cloudflare.ZoneId
	var resolvedDomain string

	if zoneID == "" && record.Spec.Cloudflare.Domain != "" {
		// Priority 2: Use explicit domain from spec to find CloudflareDomain
		domainInfo, err := r.domainResolver.Resolve(ctx, record.Spec.Cloudflare.Domain)
		if err != nil {
			logger.Error(err, "Failed to resolve explicit domain", "domain", record.Spec.Cloudflare.Domain)
		} else if domainInfo != nil {
			zoneID = domainInfo.ZoneID
			accountID = domainInfo.AccountID
			resolvedDomain = domainInfo.Domain
			if credRef.Name == "" && domainInfo.CredentialsRef != nil {
				credRef.Name = domainInfo.CredentialsRef.Name
			}
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
		} else if domainInfo != nil {
			zoneID = domainInfo.ZoneID
			accountID = domainInfo.AccountID
			resolvedDomain = domainInfo.Domain
			if credRef.Name == "" && domainInfo.CredentialsRef != nil {
				credRef.Name = domainInfo.CredentialsRef.Name
			}
			logger.V(1).Info("Resolved Zone ID via CloudflareDomain (name suffix match)",
				"name", record.Spec.Name,
				"domain", domainInfo.Domain,
				"zoneId", zoneID)
		}
	}

	if zoneID == "" {
		return "", "", credRef, fmt.Errorf(
			"unable to determine Zone ID for DNS record %s: specify cloudflare.zoneId or create a CloudflareDomain resource",
			record.Spec.Name)
	}

	// Validate that the record name belongs to the resolved domain
	if resolvedDomain != "" {
		if err := validateRecordBelongsToDomain(record.Spec.Name, resolvedDomain); err != nil {
			return "", "", credRef, fmt.Errorf(
				"DNS record validation failed: %w. "+
					"Hint: create a CloudflareDomain resource for the correct domain or specify cloudflare.zoneId explicitly",
				err)
		}
	}

	return zoneID, accountID, credRef, nil
}

// handleDeletion handles the deletion of a DNSRecord.
// It deletes the record from Cloudflare and unregisters from SyncState.
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *DNSRecordReconciler) handleDeletion(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(dnsRecord, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete from Cloudflare if record exists
	effectiveZoneID := dnsRecord.Status.ZoneID
	if effectiveZoneID == "" {
		effectiveZoneID = zoneID
	}

	if dnsRecord.Status.RecordID != "" && effectiveZoneID != "" {
		// Create API client for deletion
		cfCredRef := &networkingv1alpha2.CloudflareCredentialsRef{Name: credRef.Name}
		apiClient, err := cf.NewAPIClientFromCredentialsRef(ctx, r.Client, cfCredRef)
		if err != nil {
			logger.Error(err, "Failed to create API client for deletion")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		logger.Info("Deleting DNS Record from Cloudflare",
			"recordId", dnsRecord.Status.RecordID,
			"zoneId", effectiveZoneID)

		if err := apiClient.DeleteDNSRecordInZone(effectiveZoneID, dnsRecord.Status.RecordID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to delete DNS Record from Cloudflare")
				r.Recorder.Event(dnsRecord, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete from Cloudflare: %s", cf.SanitizeErrorMessage(err)))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			logger.Info("DNS Record already deleted from Cloudflare")
			r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, "AlreadyDeleted",
				"DNS Record was already deleted from Cloudflare")
		} else {
			r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, "Deleted", "Deleted from Cloudflare")
		}
	}

	// Unregister from SyncState
	source := service.Source{
		Kind:      "DNSRecord",
		Namespace: dnsRecord.Namespace,
		Name:      dnsRecord.Name,
	}
	if err := r.dnsService.Unregister(ctx, dnsRecord.Status.RecordID, source); err != nil {
		logger.Error(err, "Failed to unregister DNS record from SyncState")
		// Non-fatal - continue with finalizer removal
	}

	// Remove finalizer with retry logic
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		controllerutil.RemoveFinalizer(dnsRecord, FinalizerName)
	}); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// registerDNSRecord registers the DNS record configuration to SyncState.
// The actual sync to Cloudflare is handled by DNSSyncController.
func (r *DNSRecordReconciler) registerDNSRecord(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneID, accountID string,
	credRef networkingv1alpha2.CredentialsReference,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Build DNS record configuration
	config := dnssvc.DNSRecordConfig{
		Name:    dnsRecord.Spec.Name,
		Type:    dnsRecord.Spec.Type,
		Content: dnsRecord.Spec.Content,
		TTL:     dnsRecord.Spec.TTL,
		Proxied: dnsRecord.Spec.Proxied,
		Comment: dnsRecord.Spec.Comment,
		Tags:    dnsRecord.Spec.Tags,
	}

	if dnsRecord.Spec.Priority != nil {
		config.Priority = dnsRecord.Spec.Priority
	}

	// Build data for special record types
	if dnsRecord.Spec.Data != nil {
		config.Data = r.buildRecordData(dnsRecord.Spec.Data)
	}

	// Create source reference
	source := service.Source{
		Kind:      "DNSRecord",
		Namespace: dnsRecord.Namespace,
		Name:      dnsRecord.Name,
	}

	// Register to SyncState
	opts := dnssvc.RegisterOptions{
		ZoneID:         zoneID,
		AccountID:      accountID,
		RecordID:       dnsRecord.Status.RecordID, // May be empty for new records
		Source:         source,
		Config:         config,
		CredentialsRef: credRef,
	}

	if err := r.dnsService.Register(ctx, opts); err != nil {
		logger.Error(err, "Failed to register DNS record configuration")
		r.Recorder.Event(dnsRecord, corev1.EventTypeWarning, "RegisterFailed",
			fmt.Sprintf("Failed to register DNS record: %s", err.Error()))
		return r.updateStatusError(ctx, dnsRecord, err)
	}

	r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, "Registered",
		fmt.Sprintf("Registered DNS Record '%s' configuration to SyncState", dnsRecord.Spec.Name))

	// Update status to Pending - actual sync happens via DNSSyncController
	return r.updateStatusPending(ctx, dnsRecord, zoneID)
}

// buildRecordData converts DNSRecordData from API type to service layer type.
func (*DNSRecordReconciler) buildRecordData(data *networkingv1alpha2.DNSRecordData) *dnssvc.DNSRecordData {
	if data == nil {
		return nil
	}

	return &dnssvc.DNSRecordData{
		// SRV record data
		Service: data.Service,
		Proto:   data.Proto,
		Weight:  data.Weight,
		Port:    data.Port,
		Target:  data.Target,

		// CAA record data
		Flags: data.Flags,
		Tag:   data.Tag,
		Value: data.Value,

		// CERT/SSHFP/TLSA record data
		Algorithm:    data.Algorithm,
		Certificate:  data.Certificate,
		KeyTag:       data.KeyTag,
		Usage:        data.Usage,
		Selector:     data.Selector,
		MatchingType: data.MatchingType,

		// LOC record data
		LatDegrees:    data.LatDegrees,
		LatMinutes:    data.LatMinutes,
		LatSeconds:    data.LatSeconds,
		LatDirection:  data.LatDirection,
		LongDegrees:   data.LongDegrees,
		LongMinutes:   data.LongMinutes,
		LongSeconds:   data.LongSeconds,
		LongDirection: data.LongDirection,
		Altitude:      data.Altitude,
		Size:          data.Size,
		PrecisionHorz: data.PrecisionHorz,
		PrecisionVert: data.PrecisionVert,

		// URI record data
		ContentURI: data.ContentURI,
	}
}

func (r *DNSRecordReconciler) updateStatusError(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		dnsRecord.Status.State = "Error"
		meta.SetStatusCondition(&dnsRecord.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: dnsRecord.Generation,
			Reason:             "ReconcileError",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		dnsRecord.Status.ObservedGeneration = dnsRecord.Generation
	})

	if updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", updateErr)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DNSRecordReconciler) updateStatusPending(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneID string,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		// Keep existing RecordID and FQDN if available
		if dnsRecord.Status.ZoneID == "" {
			dnsRecord.Status.ZoneID = zoneID
		}
		dnsRecord.Status.State = "Pending"
		meta.SetStatusCondition(&dnsRecord.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: dnsRecord.Generation,
			Reason:             "Pending",
			Message:            "DNS record configuration registered, waiting for sync",
			LastTransitionTime: metav1.Now(),
		})
		dnsRecord.Status.ObservedGeneration = dnsRecord.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue to check for sync completion
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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
func validateRecordBelongsToDomain(recordName, domainName string) error {
	// Normalize: remove trailing dots (FQDN format)
	recordName = strings.TrimSuffix(recordName, ".")
	domainName = strings.TrimSuffix(domainName, ".")

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

	// Initialize DNSService
	r.dnsService = dnssvc.NewService(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DNSRecord{}).
		Watches(&networkingv1alpha2.CloudflareDomain{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForDomain)).
		Complete(r)
}
