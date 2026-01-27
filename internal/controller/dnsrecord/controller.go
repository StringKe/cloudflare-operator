// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package dnsrecord implements the Controller for DNSRecord CRD.
// This controller directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture (CRD → Controller → CF API).
package dnsrecord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/controller/common"
	"github.com/StringKe/cloudflare-operator/internal/resolver"
	"github.com/StringKe/cloudflare-operator/internal/resolver/address"
)

const (
	FinalizerName = "dnsrecord.networking.cloudflare-operator.io/finalizer"
)

// DNSRecordReconciler reconciles a DNSRecord object.
// It directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture.
type DNSRecordReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	APIFactory      *common.APIClientFactory
	domainResolver  *resolver.DomainResolver
	addressResolver *address.Resolver
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch

//nolint:revive // cognitive complexity is acceptable for this reconcile loop
func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DNSRecord instance
	dnsRecord := &networkingv1alpha2.DNSRecord{}
	if err := r.Get(ctx, req.NamespacedName, dnsRecord); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resolve Zone ID and credentials
	zoneInfo, err := r.resolveZoneAndCredentials(ctx, dnsRecord)
	if err != nil {
		logger.Error(err, "Failed to resolve zone and credentials")
		return r.setErrorStatus(ctx, dnsRecord, err)
	}

	// Handle deletion
	if !dnsRecord.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, dnsRecord, zoneInfo)
	}

	// Get API client
	apiResult, err := r.getAPIClient(ctx, dnsRecord, zoneInfo)
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.setErrorStatus(ctx, dnsRecord, err)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(dnsRecord, FinalizerName) {
		controllerutil.AddFinalizer(dnsRecord, FinalizerName)
		if err := r.Update(ctx, dnsRecord); err != nil {
			return ctrl.Result{}, err
		}
		// Re-fetch to get updated version
		if err := r.Get(ctx, req.NamespacedName, dnsRecord); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve content from source if using dynamic mode
	resolvedInfo, err := r.resolveContent(ctx, dnsRecord)
	if err != nil {
		logger.Error(err, "Failed to resolve content from source")
		return r.setErrorStatus(ctx, dnsRecord, err)
	}

	// Handle source deletion based on policy
	if resolvedInfo != nil && !resolvedInfo.SourceExists {
		return r.handleSourceDeleted(ctx, dnsRecord)
	}

	// Sync DNS record to Cloudflare
	return r.syncDNSRecord(ctx, dnsRecord, zoneInfo, apiResult, resolvedInfo)
}

// zoneResolutionInfo contains the resolved zone information.
type zoneResolutionInfo struct {
	ZoneID    string
	AccountID string
	CredRef   networkingv1alpha2.CredentialsReference
	Domain    string
}

// resolveZoneAndCredentials resolves the Zone ID, Account ID, and credentials reference.
//
//nolint:revive // cognitive complexity is acceptable for this resolution function
func (r *DNSRecordReconciler) resolveZoneAndCredentials(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
) (*zoneResolutionInfo, error) {
	logger := log.FromContext(ctx)
	info := &zoneResolutionInfo{}

	// Get credentials reference
	if dnsRecord.Spec.Cloudflare.CredentialsRef != nil {
		info.CredRef = networkingv1alpha2.CredentialsReference{
			Name: dnsRecord.Spec.Cloudflare.CredentialsRef.Name,
		}
	}

	// Determine Zone ID using priority:
	// 1. Explicit ZoneId in spec.cloudflare.zoneId
	// 2. Explicit Domain in spec.cloudflare.domain -> resolve via CloudflareDomain
	// 3. Use DomainResolver to find CloudflareDomain matching the record name
	info.ZoneID = dnsRecord.Spec.Cloudflare.ZoneId

	if info.ZoneID == "" && dnsRecord.Spec.Cloudflare.Domain != "" {
		// Priority 2: Use explicit domain from spec to find CloudflareDomain
		domainInfo, err := r.domainResolver.Resolve(ctx, dnsRecord.Spec.Cloudflare.Domain)
		if err != nil {
			logger.Error(err, "Failed to resolve explicit domain", "domain", dnsRecord.Spec.Cloudflare.Domain)
		} else if domainInfo != nil {
			info.ZoneID = domainInfo.ZoneID
			info.AccountID = domainInfo.AccountID
			info.Domain = domainInfo.Domain
			if info.CredRef.Name == "" && domainInfo.CredentialsRef != nil {
				info.CredRef.Name = domainInfo.CredentialsRef.Name
			}
			logger.V(1).Info("Resolved Zone ID via explicit spec.cloudflare.domain",
				"name", dnsRecord.Spec.Name,
				"specDomain", dnsRecord.Spec.Cloudflare.Domain,
				"resolvedDomain", domainInfo.Domain,
				"zoneId", info.ZoneID)
		}
	}

	if info.ZoneID == "" {
		// Priority 3: Try DomainResolver using record name suffix matching
		domainInfo, err := r.domainResolver.Resolve(ctx, dnsRecord.Spec.Name)
		if err != nil {
			logger.Error(err, "Failed to resolve domain for DNS record", "name", dnsRecord.Spec.Name)
		} else if domainInfo != nil {
			info.ZoneID = domainInfo.ZoneID
			info.AccountID = domainInfo.AccountID
			info.Domain = domainInfo.Domain
			if info.CredRef.Name == "" && domainInfo.CredentialsRef != nil {
				info.CredRef.Name = domainInfo.CredentialsRef.Name
			}
			logger.V(1).Info("Resolved Zone ID via CloudflareDomain (name suffix match)",
				"name", dnsRecord.Spec.Name,
				"domain", domainInfo.Domain,
				"zoneId", info.ZoneID)
		}
	}

	if info.ZoneID == "" {
		return nil, fmt.Errorf(
			"unable to determine Zone ID for DNS record %s: specify cloudflare.zoneId or create a CloudflareDomain resource",
			dnsRecord.Spec.Name)
	}

	// Validate that the record name belongs to the resolved domain
	if info.Domain != "" {
		if err := validateRecordBelongsToDomain(dnsRecord.Spec.Name, info.Domain); err != nil {
			return nil, fmt.Errorf(
				"DNS record validation failed: %w. "+
					"Hint: create a CloudflareDomain resource for the correct domain or specify cloudflare.zoneId explicitly",
				err)
		}
	}

	return info, nil
}

// getAPIClient returns a Cloudflare API client for the DNS record.
func (r *DNSRecordReconciler) getAPIClient(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneInfo *zoneResolutionInfo,
) (*common.APIClientResult, error) {
	opts := common.APIClientOptions{
		Namespace:      dnsRecord.Namespace,
		StatusZoneID:   zoneInfo.ZoneID,
		CredentialsRef: &zoneInfo.CredRef,
	}

	// If CloudflareDetails is specified, use it
	if dnsRecord.Spec.Cloudflare.CredentialsRef != nil {
		opts.CloudflareDetails = &networkingv1alpha2.CloudflareDetails{
			CredentialsRef: dnsRecord.Spec.Cloudflare.CredentialsRef,
			ZoneId:         zoneInfo.ZoneID,
			AccountId:      zoneInfo.AccountID,
		}
	}

	return r.APIFactory.GetClient(ctx, opts)
}

// resolvedContentInfo contains the resolved content information.
type resolvedContentInfo struct {
	// Content is the resolved content string (IP or hostname).
	Content string
	// Type is the resolved record type (A, AAAA, CNAME).
	Type string
	// Addresses contains all resolved addresses.
	Addresses []address.ResolvedAddress
	// SourceResourceVersion is the source resource's version.
	SourceResourceVersion string
	// SourceExists indicates if the source resource exists.
	SourceExists bool
}

// resolveContent resolves content from sourceRef if specified.
func (r *DNSRecordReconciler) resolveContent(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
) (*resolvedContentInfo, error) {
	if dnsRecord.Spec.SourceRef == nil {
		// Static mode - no resolution needed
		return nil, nil
	}

	result, err := r.addressResolver.Resolve(ctx, dnsRecord.Spec.SourceRef, dnsRecord.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve source: %w", err)
	}

	if !result.SourceExists {
		return &resolvedContentInfo{SourceExists: false}, nil
	}

	if len(result.Addresses) == 0 {
		return nil, errors.New("source resource has no addresses")
	}

	// Apply address selection policy
	selected := address.SelectAddresses(result.Addresses, dnsRecord.Spec.AddressSelection)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no addresses selected after applying policy %s", dnsRecord.Spec.AddressSelection)
	}

	// Determine record type
	recordType := address.DetermineRecordType(selected[0])
	if dnsRecord.Spec.Type != "" {
		recordType = dnsRecord.Spec.Type // Allow override
	}

	return &resolvedContentInfo{
		Content:               selected[0].Value,
		Type:                  recordType,
		Addresses:             selected,
		SourceResourceVersion: result.ResourceVersion,
		SourceExists:          true,
	}, nil
}

// handleSourceDeleted handles the case when the source resource is deleted.
func (r *DNSRecordReconciler) handleSourceDeleted(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	policy := dnsRecord.Spec.SourceDeletionPolicy
	if policy == "" {
		policy = networkingv1alpha2.SourceDeletionDelete
	}

	if policy == networkingv1alpha2.SourceDeletionOrphan {
		logger.Info("Source resource deleted, orphaning DNS record per policy",
			"name", dnsRecord.Spec.Name,
			"policy", policy)
		// Update status to reflect orphaned state
		return r.setOrphanedStatus(ctx, dnsRecord)
	}

	// Default: Delete the DNS record
	logger.Info("Source resource deleted, deleting DNS record per policy",
		"name", dnsRecord.Spec.Name,
		"policy", policy)

	// Delete the DNSRecord CR (which will trigger handleDeletion via finalizer)
	if err := r.Delete(ctx, dnsRecord); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete DNSRecord: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// setOrphanedStatus updates status when source is deleted but record is orphaned.
func (r *DNSRecordReconciler) setOrphanedStatus(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		dnsRecord.Status.State = "Orphaned"
		meta.SetStatusCondition(&dnsRecord.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: dnsRecord.Generation,
			Reason:             "SourceDeleted",
			Message:            "Source resource deleted, DNS record orphaned per policy",
			LastTransitionTime: metav1.Now(),
		})
		dnsRecord.Status.ObservedGeneration = dnsRecord.Generation
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	return ctrl.Result{}, nil
}

// syncDNSRecord syncs the DNS record to Cloudflare.
//
//nolint:revive // cognitive complexity acceptable for sync logic
func (r *DNSRecordReconciler) syncDNSRecord(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneInfo *zoneResolutionInfo,
	apiResult *common.APIClientResult,
	resolvedInfo *resolvedContentInfo,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	api := apiResult.API

	// Determine content and type
	var content, recordType string
	if resolvedInfo != nil {
		// Dynamic mode - use resolved content
		content = resolvedInfo.Content
		recordType = resolvedInfo.Type
		logger.V(1).Info("Using resolved content from source",
			"content", content,
			"type", recordType,
			"sourceType", dnsRecord.Spec.SourceRef.GetSourceType())
	} else {
		// Static mode - use spec values
		content = dnsRecord.Spec.Content
		recordType = dnsRecord.Spec.Type
	}

	// Build DNS record params
	params := cf.DNSRecordParams{
		Name:    dnsRecord.Spec.Name,
		Type:    recordType,
		Content: content,
		TTL:     dnsRecord.Spec.TTL,
		Proxied: dnsRecord.Spec.Proxied,
		Comment: dnsRecord.Spec.Comment,
		Tags:    dnsRecord.Spec.Tags,
	}

	if dnsRecord.Spec.Priority != nil {
		params.Priority = dnsRecord.Spec.Priority
	}

	if dnsRecord.Spec.Data != nil {
		params.Data = r.buildRecordData(dnsRecord.Spec.Data)
	}

	var result *cf.DNSRecordResult
	var err error

	if dnsRecord.Status.RecordID != "" {
		// Update existing record
		logger.Info("Updating DNS record",
			"recordId", dnsRecord.Status.RecordID,
			"name", params.Name,
			"type", params.Type)
		result, err = api.UpdateDNSRecordInZone(ctx, zoneInfo.ZoneID, dnsRecord.Status.RecordID, params)
		if err != nil {
			if cf.IsNotFoundError(err) {
				// Record was deleted externally, create new one
				logger.Info("Record not found, creating new one")
				result, err = api.CreateDNSRecordInZone(ctx, zoneInfo.ZoneID, params)
			}
		}
	} else {
		// Check if record already exists (adoption scenario)
		existingID, lookupErr := api.GetDNSRecordIDInZone(ctx, zoneInfo.ZoneID, dnsRecord.Spec.Name, recordType)
		if lookupErr != nil && !cf.IsNotFoundError(lookupErr) {
			logger.Error(lookupErr, "Failed to check for existing DNS record")
			return r.setErrorStatus(ctx, dnsRecord, lookupErr)
		}

		if existingID != "" {
			// Adopt existing record
			logger.Info("Adopting existing DNS record", "recordId", existingID)
			result, err = api.UpdateDNSRecordInZone(ctx, zoneInfo.ZoneID, existingID, params)
		} else {
			// Create new record
			logger.Info("Creating DNS record", "name", params.Name, "type", params.Type)
			result, err = api.CreateDNSRecordInZone(ctx, zoneInfo.ZoneID, params)
		}
	}

	if err != nil {
		logger.Error(err, "Failed to sync DNS record")
		r.Recorder.Event(dnsRecord, corev1.EventTypeWarning, controller.EventReasonSyncFailed,
			cf.SanitizeErrorMessage(err))
		return r.setErrorStatus(ctx, dnsRecord, err)
	}

	// Update status with result
	return r.setSuccessStatus(ctx, dnsRecord, zoneInfo.ZoneID, result, resolvedInfo)
}

// handleDeletion handles the deletion of a DNSRecord.
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *DNSRecordReconciler) handleDeletion(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneInfo *zoneResolutionInfo,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(dnsRecord, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Delete DNS record from Cloudflare
	if dnsRecord.Status.RecordID != "" && zoneInfo.ZoneID != "" {
		apiResult, err := r.getAPIClient(ctx, dnsRecord, zoneInfo)
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal - record might not exist anyway
		} else {
			if err := apiResult.API.DeleteDNSRecordInZone(ctx, zoneInfo.ZoneID, dnsRecord.Status.RecordID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to delete DNS record from Cloudflare")
					r.Recorder.Event(dnsRecord, corev1.EventTypeWarning, controller.EventReasonDeleteFailed,
						cf.SanitizeErrorMessage(err))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, err
				}
				// Record already deleted, continue with finalizer removal
			} else {
				r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, controller.EventReasonDeleted,
					fmt.Sprintf("Deleted DNS record %s from Cloudflare", dnsRecord.Status.RecordID))
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		controllerutil.RemoveFinalizer(dnsRecord, FinalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

// setSuccessStatus updates the DNS record status to indicate success.
func (r *DNSRecordReconciler) setSuccessStatus(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneID string,
	result *cf.DNSRecordResult,
	resolvedInfo *resolvedContentInfo,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, dnsRecord, func() {
		dnsRecord.Status.ZoneID = zoneID
		dnsRecord.Status.RecordID = result.ID
		dnsRecord.Status.FQDN = result.Name
		dnsRecord.Status.State = "Active"
		meta.SetStatusCondition(&dnsRecord.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: dnsRecord.Generation,
			Reason:             "Synced",
			Message:            "DNS record synced to Cloudflare",
			LastTransitionTime: metav1.Now(),
		})
		dnsRecord.Status.ObservedGeneration = dnsRecord.Generation

		// Update source-specific status fields
		if resolvedInfo != nil {
			dnsRecord.Status.ResolvedType = resolvedInfo.Type
			dnsRecord.Status.ResolvedContent = resolvedInfo.Content
			dnsRecord.Status.ResolvedAddresses = address.AddressesToStrings(resolvedInfo.Addresses)
			dnsRecord.Status.SourceResourceVersion = resolvedInfo.SourceResourceVersion
			dnsRecord.Status.ManagedRecordIDs = []string{result.ID}
		} else {
			// Clear source-specific fields for static mode
			dnsRecord.Status.ResolvedType = ""
			dnsRecord.Status.ResolvedContent = ""
			dnsRecord.Status.ResolvedAddresses = nil
			dnsRecord.Status.SourceResourceVersion = ""
			dnsRecord.Status.ManagedRecordIDs = nil
		}
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	r.Recorder.Event(dnsRecord, corev1.EventTypeNormal, controller.EventReasonSynced,
		fmt.Sprintf("DNS record %s synced to Cloudflare (ID: %s)", dnsRecord.Spec.Name, result.ID))

	return ctrl.Result{}, nil
}

// setErrorStatus updates the DNS record status to indicate an error.
func (r *DNSRecordReconciler) setErrorStatus(
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

	r.Recorder.Event(dnsRecord, corev1.EventTypeWarning, controller.EventReasonReconcileFailed,
		cf.SanitizeErrorMessage(err))

	// Determine requeue based on error type
	if cf.IsPermanentError(err) {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// buildRecordData converts DNSRecordData from API type to cf type.
func (*DNSRecordReconciler) buildRecordData(data *networkingv1alpha2.DNSRecordData) *cf.DNSRecordDataParams {
	if data == nil {
		return nil
	}

	return &cf.DNSRecordDataParams{
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

// findDNSRecordsForDomain returns DNSRecords that may need reconciliation when a CloudflareDomain changes.
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
	for _, dnsRecord := range recordList.Items {
		// Check if this record's name is a suffix of the domain or equals the domain
		if dnsRecord.Spec.Name == domain.Spec.Domain ||
			len(dnsRecord.Spec.Name) > len(domain.Spec.Domain) &&
				dnsRecord.Spec.Name[len(dnsRecord.Spec.Name)-len(domain.Spec.Domain)-1:] == "."+domain.Spec.Domain {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&dnsRecord),
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
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), mgr.GetLogger())
	r.domainResolver = resolver.NewDomainResolver(mgr.GetClient(), logr.Discard())
	r.addressResolver = address.NewResolver(mgr.GetClient())

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DNSRecord{}).
		Watches(&networkingv1alpha2.CloudflareDomain{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForDomain)).
		// Watch source resources for dynamic mode
		Watches(&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForService)).
		Watches(&networkingv1.Ingress{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForIngress)).
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForGateway)).
		Watches(&gatewayv1.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForHTTPRoute)).
		Watches(&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForNode)).
		Complete(r)
}

// sourceRefMatcher is a function that checks if a DNSRecord's sourceRef matches a given resource.
type sourceRefMatcher func(dnsRec *networkingv1alpha2.DNSRecord, objName, objNamespace string) bool

// findDNSRecordsForSource is a generic function to find DNSRecords that reference a source resource.
func (r *DNSRecordReconciler) findDNSRecordsForSource(
	ctx context.Context,
	objName, objNamespace string,
	matcher sourceRefMatcher,
) []reconcile.Request {
	recordList := &networkingv1alpha2.DNSRecordList{}
	if err := r.List(ctx, recordList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for i := range recordList.Items {
		dnsRec := &recordList.Items[i]
		if matcher(dnsRec, objName, objNamespace) {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(dnsRec),
			})
		}
	}
	return requests
}

// findDNSRecordsForService returns DNSRecords that reference the given Service.
func (r *DNSRecordReconciler) findDNSRecordsForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}
	return r.findDNSRecordsForSource(ctx, svc.Name, svc.Namespace, matchServiceSource)
}

// findDNSRecordsForIngress returns DNSRecords that reference the given Ingress.
func (r *DNSRecordReconciler) findDNSRecordsForIngress(ctx context.Context, obj client.Object) []reconcile.Request {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil
	}
	return r.findDNSRecordsForSource(ctx, ing.Name, ing.Namespace, matchIngressSource)
}

// findDNSRecordsForGateway returns DNSRecords that reference the given Gateway.
func (r *DNSRecordReconciler) findDNSRecordsForGateway(ctx context.Context, obj client.Object) []reconcile.Request {
	gw, ok := obj.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}
	return r.findDNSRecordsForSource(ctx, gw.Name, gw.Namespace, matchGatewaySource)
}

// findDNSRecordsForHTTPRoute returns DNSRecords that reference the given HTTPRoute.
func (r *DNSRecordReconciler) findDNSRecordsForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil
	}
	return r.findDNSRecordsForSource(ctx, route.Name, route.Namespace, matchHTTPRouteSource)
}

// findDNSRecordsForNode returns DNSRecords that reference the given Node.
func (r *DNSRecordReconciler) findDNSRecordsForNode(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}
	return r.findDNSRecordsForSource(ctx, node.Name, "", matchNodeSource)
}

// matchServiceSource checks if a DNSRecord references the given Service.
func matchServiceSource(dnsRec *networkingv1alpha2.DNSRecord, objName, objNamespace string) bool {
	if dnsRec.Spec.SourceRef == nil || dnsRec.Spec.SourceRef.Service == nil {
		return false
	}
	ref := dnsRec.Spec.SourceRef.Service
	ns := ref.Namespace
	if ns == "" {
		ns = dnsRec.Namespace
	}
	return ref.Name == objName && ns == objNamespace
}

// matchIngressSource checks if a DNSRecord references the given Ingress.
func matchIngressSource(dnsRec *networkingv1alpha2.DNSRecord, objName, objNamespace string) bool {
	if dnsRec.Spec.SourceRef == nil || dnsRec.Spec.SourceRef.Ingress == nil {
		return false
	}
	ref := dnsRec.Spec.SourceRef.Ingress
	ns := ref.Namespace
	if ns == "" {
		ns = dnsRec.Namespace
	}
	return ref.Name == objName && ns == objNamespace
}

// matchGatewaySource checks if a DNSRecord references the given Gateway.
func matchGatewaySource(dnsRec *networkingv1alpha2.DNSRecord, objName, objNamespace string) bool {
	if dnsRec.Spec.SourceRef == nil || dnsRec.Spec.SourceRef.Gateway == nil {
		return false
	}
	ref := dnsRec.Spec.SourceRef.Gateway
	ns := ref.Namespace
	if ns == "" {
		ns = dnsRec.Namespace
	}
	return ref.Name == objName && ns == objNamespace
}

// matchHTTPRouteSource checks if a DNSRecord references the given HTTPRoute.
func matchHTTPRouteSource(dnsRec *networkingv1alpha2.DNSRecord, objName, objNamespace string) bool {
	if dnsRec.Spec.SourceRef == nil || dnsRec.Spec.SourceRef.HTTPRoute == nil {
		return false
	}
	ref := dnsRec.Spec.SourceRef.HTTPRoute
	ns := ref.Namespace
	if ns == "" {
		ns = dnsRec.Namespace
	}
	return ref.Name == objName && ns == objNamespace
}

// matchNodeSource checks if a DNSRecord references the given Node.
func matchNodeSource(dnsRec *networkingv1alpha2.DNSRecord, objName, _ string) bool {
	if dnsRec.Spec.SourceRef == nil || dnsRec.Spec.SourceRef.Node == nil {
		return false
	}
	return dnsRec.Spec.SourceRef.Node.Name == objName
}
