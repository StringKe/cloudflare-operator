// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package dnsrecord implements the Controller for DNSRecord CRD.
// This controller directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture (CRD → Controller → CF API).
package dnsrecord

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
	"github.com/StringKe/cloudflare-operator/internal/resolver"
)

const (
	FinalizerName = "dnsrecord.networking.cloudflare-operator.io/finalizer"
)

// DNSRecordReconciler reconciles a DNSRecord object.
// It directly calls Cloudflare API and writes status back to CRD,
// following the simplified 3-layer architecture.
type DNSRecordReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	APIFactory     *common.APIClientFactory
	domainResolver *resolver.DomainResolver
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=dnsrecords/finalizers,verbs=update

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

	// Sync DNS record to Cloudflare
	return r.syncDNSRecord(ctx, dnsRecord, zoneInfo, apiResult)
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

// syncDNSRecord syncs the DNS record to Cloudflare.
//
//nolint:revive // cognitive complexity acceptable for sync logic
func (r *DNSRecordReconciler) syncDNSRecord(
	ctx context.Context,
	dnsRecord *networkingv1alpha2.DNSRecord,
	zoneInfo *zoneResolutionInfo,
	apiResult *common.APIClientResult,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	api := apiResult.API

	// Build DNS record params
	params := cf.DNSRecordParams{
		Name:    dnsRecord.Spec.Name,
		Type:    dnsRecord.Spec.Type,
		Content: dnsRecord.Spec.Content,
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
		existingID, lookupErr := api.GetDNSRecordIDInZone(ctx, zoneInfo.ZoneID, dnsRecord.Spec.Name, dnsRecord.Spec.Type)
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
	return r.setSuccessStatus(ctx, dnsRecord, zoneInfo.ZoneID, result)
}

// handleDeletion handles the deletion of a DNSRecord.
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.DNSRecord{}).
		Watches(&networkingv1alpha2.CloudflareDomain{},
			handler.EnqueueRequestsFromMapFunc(r.findDNSRecordsForDomain)).
		Complete(r)
}
