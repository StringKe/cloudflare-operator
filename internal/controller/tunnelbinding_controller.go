// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/service"
	tunnelsvc "github.com/StringKe/cloudflare-operator/internal/service/tunnel"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha1 "github.com/StringKe/cloudflare-operator/api/v1alpha1"
	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"

	"k8s.io/client-go/tools/record"
)

const (
	tunnelKindClusterTunnel = "clustertunnel"
	tunnelKindTunnel        = "tunnel"
)

// TunnelBindingReconciler reconciles a TunnelBinding object
type TunnelBindingReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Recorder           record.EventRecorder
	Namespace          string
	OverwriteUnmanaged bool

	// Custom data for ease of (re)use

	ctx            context.Context
	log            logr.Logger
	binding        *networkingv1alpha1.TunnelBinding
	tunnelID       string
	fallbackTarget string
	warpRouting    bool

	// Resolved credentials data (following Unified Sync Architecture - no cfAPI field)
	accountID        string
	domain           string
	cloudflareConfig networkingv1alpha2.CloudflareDetails // Used for creating temporary API clients
}

// labelsForBinding returns the labels for selecting the Bindings served by a Tunnel.
func labelsForBinding(binding networkingv1alpha1.TunnelBinding) map[string]string {
	labels := map[string]string{
		tunnelNameLabel: binding.TunnelRef.Name,
		tunnelKindLabel: binding.Kind,
	}

	return labels
}

func (r *TunnelBindingReconciler) initStruct(ctx context.Context, tunnelBinding *networkingv1alpha1.TunnelBinding) error {
	r.ctx = ctx
	r.binding = tunnelBinding

	// Process based on Tunnel Kind
	switch strings.ToLower(r.binding.TunnelRef.Kind) {
	case tunnelKindClusterTunnel:
		namespacedName := apitypes.NamespacedName{Name: r.binding.TunnelRef.Name, Namespace: r.Namespace}
		clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
		if err := r.Get(r.ctx, namespacedName, clusterTunnel); err != nil {
			r.log.Error(err, "Failed to get ClusterTunnel", "namespacedName", namespacedName)
			r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrTunnel", "Error getting ClusterTunnel")
			return err
		}

		r.fallbackTarget = clusterTunnel.Spec.FallbackTarget
		r.tunnelID = clusterTunnel.Status.TunnelId
		r.warpRouting = clusterTunnel.Spec.EnableWarpRouting
		r.cloudflareConfig = clusterTunnel.Spec.Cloudflare

		// Resolve credentials to get accountID and domain
		if err := r.resolveCredentials(clusterTunnel.Spec, clusterTunnel.Status, r.Namespace); err != nil {
			r.log.Error(err, "unable to resolve credentials")
			r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrApiConfig", "Error resolving credentials")
			return err
		}
	case tunnelKindTunnel:
		namespacedName := apitypes.NamespacedName{Name: r.binding.TunnelRef.Name, Namespace: r.binding.Namespace}
		tunnel := &networkingv1alpha2.Tunnel{}
		if err := r.Get(r.ctx, namespacedName, tunnel); err != nil {
			r.log.Error(err, "Failed to get Tunnel", "namespacedName", namespacedName)
			r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrTunnel", "Error getting Tunnel")
			return err
		}

		r.fallbackTarget = tunnel.Spec.FallbackTarget
		r.tunnelID = tunnel.Status.TunnelId
		r.warpRouting = tunnel.Spec.EnableWarpRouting
		r.cloudflareConfig = tunnel.Spec.Cloudflare

		// Resolve credentials to get accountID and domain
		if err := r.resolveCredentials(tunnel.Spec, tunnel.Status, r.binding.Namespace); err != nil {
			r.log.Error(err, "unable to resolve credentials")
			r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrApiConfig", "Error resolving credentials")
			return err
		}
	default:
		err := errors.New("invalid kind")
		r.log.Error(err, "unsupported tunnelRef Kind")
		r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrTunnelKind", "Unsupported tunnel kind")
		return err
	}

	// Check if tunnel ID is available
	if r.tunnelID == "" {
		err := errors.New("tunnel ID not available, tunnel may not be ready")
		r.log.Error(err, "tunnel ID not found in status")
		r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "ErrTunnelNotReady", "Tunnel ID not available")
		return err
	}

	return nil
}

// resolveCredentials resolves the accountID and domain from the tunnel spec and status.
// This follows the Unified Sync Architecture pattern - no cfAPI field stored in struct.
func (r *TunnelBindingReconciler) resolveCredentials(
	spec networkingv1alpha2.TunnelSpec, status networkingv1alpha2.TunnelStatus, namespace string,
) error {
	// Create a temporary API client to get credentials info
	api, _, err := getAPIDetails(r.ctx, r.Client, r.log, spec, status, namespace)
	if err != nil {
		return fmt.Errorf("failed to get API details: %w", err)
	}

	r.accountID = api.ValidAccountId
	r.domain = api.Domain

	return nil
}

// createTemporaryAPIClient creates a temporary Cloudflare API client for DNS operations.
// This is a transitional pattern following Unified Sync Architecture - the API client
// is only created when needed and not stored in the struct.
func (r *TunnelBindingReconciler) createTemporaryAPIClient() (*cf.API, error) {
	// Determine the namespace for credentials lookup
	namespace := r.binding.Namespace
	if strings.ToLower(r.binding.TunnelRef.Kind) == tunnelKindClusterTunnel {
		namespace = r.Namespace
	}

	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, namespace, r.cloudflareConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	// Set the accountID and domain from resolved values
	api.ValidAccountId = r.accountID
	api.Domain = r.domain

	return api, nil
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnelbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnels,verbs=get
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=tunnels/status,verbs=get
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=clustertunnels,verbs=get
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=clustertunnels/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TunnelBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log = ctrllog.FromContext(ctx)

	// Fetch TunnelBinding from API
	tunnelBinding := &networkingv1alpha1.TunnelBinding{}
	if err := r.Get(ctx, req.NamespacedName, tunnelBinding); err != nil {
		if apierrors.IsNotFound(err) {
			// TunnelBinding object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.log.Info("TunnelBinding deleted, updating config")
			if err = r.configureCloudflareDaemon(); err != nil {
				r.log.Error(err, "unable to update config")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch TunnelBinding")
		return ctrl.Result{}, err
	}

	// Emit deprecation warning for TunnelBinding
	r.log.Info("WARNING: TunnelBinding is deprecated and will be removed in a future release. " +
		"Please migrate to Ingress with TunnelIngressClassConfig or Gateway API (HTTPRoute, TCPRoute, UDPRoute).")
	r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "Deprecated",
		"TunnelBinding is deprecated. Please migrate to Ingress with TunnelIngressClassConfig or Gateway API.")

	if err := r.initStruct(ctx, tunnelBinding); err != nil {
		r.log.Error(err, "initialization failed")
		return ctrl.Result{}, err
	}

	// Check if TunnelBinding is marked for deletion
	if r.binding.GetDeletionTimestamp() != nil {
		// Requeue to update configmap above
		return ctrl.Result{RequeueAfter: time.Second}, r.deletionLogic()
	}

	removedHostnames, err := r.setStatus()
	if err != nil {
		return ctrl.Result{}, err
	}

	// Clean up DNS for removed hostnames (PR #166 fix)
	if len(removedHostnames) > 0 && !r.binding.TunnelRef.DisableDNSUpdates {
		if err := r.cleanupRemovedDNS(removedHostnames); err != nil {
			r.log.Error(err, "Failed to cleanup some removed DNS entries")
			r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "PartialDNSCleanup", "Some removed DNS entries failed to clean up")
			// Don't return error - continue with configuration
		}
	}

	// Sync configuration to Cloudflare API
	// In token mode, cloudflared pulls configuration from cloud automatically
	r.Recorder.Event(tunnelBinding, corev1.EventTypeNormal, "Configuring", "Syncing configuration to Cloudflare API")
	if err := r.configureCloudflareDaemon(); err != nil {
		r.log.Error(err, "unable to sync tunnel configuration to API")
		r.Recorder.Event(tunnelBinding, corev1.EventTypeWarning, "FailedConfigure", "Failed to sync configuration to Cloudflare API")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(tunnelBinding, corev1.EventTypeNormal, "Configured", "Synced Cloudflare Tunnel configuration")

	if err := r.creationLogic(); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// setStatus updates the TunnelBinding status and returns removed hostnames for DNS cleanup.
// This implements the PR #166 fix using annotations instead of struct fields for concurrency safety.
func (r *TunnelBindingReconciler) setStatus() ([]string, error) {
	status := make([]networkingv1alpha1.ServiceInfo, 0, len(r.binding.Subjects))
	currentHostnames := make(map[string]struct{})
	var hostnamesStr string

	for _, sub := range r.binding.Subjects {
		hostname, target, err := r.getConfigForSubject(sub)
		if err != nil {
			r.log.Error(err, "error getting config for service", "svc", sub.Name)
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "ErrBuildConfig",
				fmt.Sprintf("Error building TunnelBinding configuration, svc: %s", sub.Name))
		}
		status = append(status, networkingv1alpha1.ServiceInfo{Hostname: hostname, Target: target})
		currentHostnames[hostname] = struct{}{}
		hostnamesStr += hostname + ","
	}

	// Get previous hostnames from annotation (concurrency-safe approach from PR #166 fix)
	var previousHostnames []string
	if r.binding.Annotations != nil {
		if prev, ok := r.binding.Annotations[tunnelPreviousHostnamesAnnotation]; ok && prev != "" {
			previousHostnames = strings.Split(prev, ",")
		}
	}

	// Compute removed hostnames (previous - current)
	var removedHostnames []string
	for _, prev := range previousHostnames {
		if prev == "" {
			continue
		}
		if _, exists := currentHostnames[prev]; !exists {
			removedHostnames = append(removedHostnames, prev)
		}
	}

	r.binding.Status.Services = status
	r.binding.Status.Hostnames = strings.TrimSuffix(hostnamesStr, ",")

	// P0 FIX: Use retry logic for status update to handle conflicts
	if err := UpdateStatusWithConflictRetry(r.ctx, r.Client, r.binding, func() {
		r.binding.Status.Services = status
		r.binding.Status.Hostnames = strings.TrimSuffix(hostnamesStr, ",")
	}); err != nil {
		r.log.Error(err, "Failed to update TunnelBinding status", "TunnelBinding.Namespace", r.binding.Namespace, "TunnelBinding.Name", r.binding.Name)
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedStatusSet", "Failed to set Tunnel status required for operation")
		return nil, err
	}

	// P0 FIX: Update annotation with retry logic for concurrency safety (PR #166 fix improvement)
	if err := UpdateWithConflictRetry(r.ctx, r.Client, r.binding, func() {
		if r.binding.Annotations == nil {
			r.binding.Annotations = make(map[string]string)
		}
		r.binding.Annotations[tunnelPreviousHostnamesAnnotation] = strings.TrimSuffix(hostnamesStr, ",")
	}); err != nil {
		r.log.Error(err, "Failed to update previous hostnames annotation")
		// P0 FIX: Return error to trigger retry - annotation is critical for DNS cleanup
		return nil, err
	}

	r.log.Info("Tunnel status is set", "status", r.binding.Status, "removedHostnames", removedHostnames)
	return removedHostnames, nil
}

func (r *TunnelBindingReconciler) deletionLogic() error {
	if controllerutil.ContainsFinalizer(r.binding, tunnelFinalizer) {
		// Run finalization logic. If the finalization logic fails,
		// don't remove the finalizer so that we can retry during the next reconciliation.

		// Unregister from SyncState before deleting DNS entries
		if r.tunnelID != "" {
			svc := tunnelsvc.NewService(r.Client)
			source := service.Source{
				Kind:      "TunnelBinding",
				Namespace: r.binding.Namespace,
				Name:      r.binding.Name,
			}

			if err := svc.Unregister(r.ctx, r.tunnelID, source); err != nil {
				r.log.Error(err, "Failed to unregister from SyncState", "tunnelId", r.tunnelID)
				// Don't block deletion on SyncState cleanup failure
			} else {
				r.log.Info("Unregistered from SyncState", "tunnelId", r.tunnelID)
			}
		}

		// P0 FIX: Aggregate all errors and only remove finalizer if ALL deletions succeed
		var errs []error
		for _, info := range r.binding.Status.Services {
			if err := r.deleteDNSLogic(info.Hostname); err != nil {
				errs = append(errs, fmt.Errorf("delete DNS %s: %w", info.Hostname, err))
			}
		}
		if len(errs) > 0 {
			aggregatedErr := errors.Join(errs...)
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FinalizerNotUnset",
				fmt.Sprintf("Not removing Finalizer due to %d errors", len(errs)))
			return aggregatedErr
		}

		// Remove tunnelFinalizer with retry logic.
		// P0 FIX: Use UpdateWithConflictRetry for safe finalizer removal
		if err := UpdateWithConflictRetry(r.ctx, r.Client, r.binding, func() {
			controllerutil.RemoveFinalizer(r.binding, tunnelFinalizer)
		}); err != nil {
			r.log.Error(err, "unable to delete Finalizer")
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedFinalizerUnset", "Failed to remove Finalizer")
			return err
		}
		r.Recorder.Event(r.binding, corev1.EventTypeNormal, "FinalizerUnset", "Finalizer removed")
	}
	// Already removed our finalizer, all good.
	return nil
}

func (r *TunnelBindingReconciler) creationLogic() error {

	// Add labels for TunnelBinding
	if r.binding.Labels == nil {
		r.binding.Labels = make(map[string]string)
	}
	for k, v := range labelsForBinding(*r.binding) {
		r.binding.Labels[k] = v
	}

	// Update TunnelBinding resource
	if err := r.Update(r.ctx, r.binding); err != nil {
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedMetaSet", "Failed to set Labels")
		return err
	}

	// Add finalizer for TunnelBinding if DNS updates are not disabled
	if r.binding.TunnelRef.DisableDNSUpdates {
		return nil
	}

	if !controllerutil.ContainsFinalizer(r.binding, tunnelFinalizer) {
		if !controllerutil.AddFinalizer(r.binding, tunnelFinalizer) {
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedMetaSet", "Failed to set Finalizer")
			return fmt.Errorf("failed to set finalizer, trying again")
		}
		// Update TunnelBinding resource
		if err := r.Update(r.ctx, r.binding); err != nil {
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedMetaSet", "Failed to set Finalizer")
			return err
		}
	}

	r.Recorder.Event(r.binding, corev1.EventTypeNormal, "MetaSet", "TunnelBinding Finalizer and Labels added")

	// P1 FIX: Use errors.Join for proper error aggregation (fixes variable name conflict with errors package)
	var errs []error
	// Create DNS entries
	for _, info := range r.binding.Status.Services {
		if err := r.createDNSLogic(info.Hostname); err != nil {
			errs = append(errs, fmt.Errorf("create DNS %s: %w", info.Hostname, err))
		}
	}
	if len(errs) > 0 {
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDNSCreatePartial",
			fmt.Sprintf("Some DNS entries failed to create (%d errors)", len(errs)))
		return errors.Join(errs...)
	}
	return nil
}

func (r *TunnelBindingReconciler) createDNSLogic(hostname string) error {
	// Create temporary API client for DNS operations (Unified Sync Architecture pattern)
	cfAPI, err := r.createTemporaryAPIClient()
	if err != nil {
		r.log.Error(err, "Failed to create API client for DNS operations")
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedApiClient", "Failed to create API client")
		return err
	}

	txtID, dnsTxtResponse, canUseDNS, err := cfAPI.GetManagedDnsTxt(hostname)
	if err != nil {
		// We should not use this entry
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedReadingTxt", "Failed to read existing TXT DNS entry")
		return err
	}
	if !canUseDNS {
		// We cannot use this entry
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedReadingTxt",
			fmt.Sprintf("FQDN already managed by Tunnel Name: %s, Id: %s", dnsTxtResponse.TunnelName, dnsTxtResponse.TunnelId))
		return err
	}
	existingID, err := cfAPI.GetDNSCNameId(hostname)
	if err != nil {
		// Real API error (not "record not found" which now returns "", nil)
		r.log.Error(err, "Failed to check existing DNS record", "hostname", hostname)
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedCheckingDns",
			fmt.Sprintf("Failed to check existing DNS record: %s", cf.SanitizeErrorMessage(err)))
		return err
	}

	// Check if a DNS record exists (existingID != "" means record found)
	if existingID != "" {
		// without a managed TXT record when we are not supposed to overwrite it
		if !r.OverwriteUnmanaged && txtID == "" {
			err := fmt.Errorf("unmanaged FQDN present")
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedReadingTxt", "FQDN present but unmanaged by Tunnel")
			return err
		}
		// To overwrite
		dnsTxtResponse.DnsId = existingID
	}

	newDNSID, err := cfAPI.InsertOrUpdateCName(hostname, dnsTxtResponse.DnsId)
	if err != nil {
		r.log.Error(err, "Failed to insert/update DNS entry", "Hostname", hostname)
		// P0 FIX: Use SanitizeErrorMessage to prevent sensitive info leakage
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedCreatingDns",
			fmt.Sprintf("Failed to insert/update DNS entry: %s", cf.SanitizeErrorMessage(err)))
		return err
	}
	if err := cfAPI.InsertOrUpdateTXT(hostname, txtID, newDNSID); err != nil {
		r.log.Error(err, "Failed to insert/update TXT entry", "Hostname", hostname)
		// P0 FIX: Use SanitizeErrorMessage to prevent sensitive info leakage
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedCreatingTxt",
			fmt.Sprintf("Failed to insert/update TXT entry: %s", cf.SanitizeErrorMessage(err)))
		if err := cfAPI.DeleteDNSId(hostname, newDNSID, dnsTxtResponse.DnsId != ""); err != nil {
			r.log.Info("Failed to delete DNS entry, left in broken state", "Hostname", hostname)
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDeletingDns", "Failed to delete DNS entry, left in broken state")
			return err
		}
		if dnsTxtResponse.DnsId != "" {
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "DeletedDns", "Deleted DNS entry, retrying")
			r.log.Info("Deleted DNS entry", "Hostname", hostname)
		} else {
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "PreventDeleteDns", "Prevented DNS entry deletion, retrying")
			r.log.Info("Did not delete DNS entry", "Hostname", hostname)
		}
		return err
	}

	r.log.Info("Inserted/Updated DNS/TXT entry")
	r.Recorder.Event(r.binding, corev1.EventTypeNormal, "CreatedDns", "Inserted/Updated DNS/TXT entry")
	return nil
}

func (r *TunnelBindingReconciler) deleteDNSLogic(hostname string) error {
	// Create temporary API client for DNS operations (Unified Sync Architecture pattern)
	cfAPI, err := r.createTemporaryAPIClient()
	if err != nil {
		r.log.Error(err, "Failed to create API client for DNS operations")
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedApiClient", "Failed to create API client")
		return err
	}

	// Delete DNS entry
	txtID, dnsTxtResponse, canUseDNS, err := cfAPI.GetManagedDnsTxt(hostname)
	if err != nil {
		// We should not use this entry
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedReadingTxt",
			"Failed to read existing TXT DNS entry, not cleaning up")
	} else if !canUseDNS {
		// We cannot use this entry.
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedReadingTxt",
			fmt.Sprintf("FQDN already managed by Tunnel Name: %s, Id: %s, not cleaning up",
				dnsTxtResponse.TunnelName, dnsTxtResponse.TunnelId))
	} else {
		if id, err := cfAPI.GetDNSCNameId(hostname); err != nil {
			r.log.Error(err, "Error fetching DNS record", "Hostname", hostname)
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDeletingDns", "Error fetching DNS record")
		} else if id != dnsTxtResponse.DnsId {
			err := fmt.Errorf("DNS ID from TXT and real DNS record does not match")
			r.log.Error(err, "DNS ID from TXT and real DNS record does not match", "Hostname", hostname)
			r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDeletingDns", "DNS/TXT ID Mismatch")
		} else {
			if err := cfAPI.DeleteDNSId(hostname, dnsTxtResponse.DnsId, true); err != nil {
				r.log.Info("Failed to delete DNS entry", "Hostname", hostname)
				// P0 FIX: Use SanitizeErrorMessage to prevent sensitive info leakage
				errMsg := fmt.Sprintf("Failed to delete DNS entry: %s", cf.SanitizeErrorMessage(err))
				r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDeletingDns", errMsg)
				return err
			}
			r.log.Info("Deleted DNS entry", "Hostname", hostname)
			r.Recorder.Event(r.binding, corev1.EventTypeNormal, "DeletedDns", "Deleted DNS entry")
			if err := cfAPI.DeleteDNSId(hostname, txtID, true); err != nil {
				// P0 FIX: Use SanitizeErrorMessage to prevent sensitive info leakage
				errMsg := fmt.Sprintf("Failed to delete TXT entry: %s", cf.SanitizeErrorMessage(err))
				r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedDeletingTxt", errMsg)
				return err
			}
			r.log.Info("Deleted DNS TXT entry", "Hostname", hostname)
			r.Recorder.Event(r.binding, corev1.EventTypeNormal, "DeletedTxt", "Deleted DNS TXT entry")
		}
	}
	return nil
}

// cleanupRemovedDNS cleans up DNS entries for hostnames that were removed from the TunnelBinding.
// This implements PR #166 fix with proper error aggregation using errors.Join.
func (r *TunnelBindingReconciler) cleanupRemovedDNS(hostnames []string) error {
	var errs []error
	for _, hostname := range hostnames {
		r.log.Info("Cleaning up removed DNS entry", "hostname", hostname)
		r.Recorder.Event(r.binding, corev1.EventTypeNormal, "CleaningUpDNS", fmt.Sprintf("Cleaning up DNS for removed hostname: %s", hostname))
		if err := r.deleteDNSLogic(hostname); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", hostname, err))
		}
	}
	return errors.Join(errs...)
}

func (r *TunnelBindingReconciler) getRelevantTunnelBindings() ([]networkingv1alpha1.TunnelBinding, error) {
	// Fetch TunnelBindings from API
	listOpts := []client.ListOption{client.MatchingLabels(map[string]string{
		tunnelNameLabel: r.binding.TunnelRef.Name,
		tunnelKindLabel: r.binding.Kind,
	})}
	tunnelBindingList := &networkingv1alpha1.TunnelBindingList{}
	if err := r.List(r.ctx, tunnelBindingList, listOpts...); err != nil {
		r.log.Error(err, "failed to list Tunnel Bindings", "listOpts", listOpts)
		return tunnelBindingList.Items, err
	}

	bindings := tunnelBindingList.Items

	if len(bindings) == 0 {
		// Is this possible? Shouldn't the one that triggered this exist?
		r.log.Info("No tunnelBindings found, tunnel not in use")
	}

	// Sort by binding name for idempotent config generation
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].Name < bindings[j].Name
	})

	return bindings, nil
}

// Get the config entry to be added for this subject
func (r TunnelBindingReconciler) getConfigForSubject(subject networkingv1alpha1.TunnelBindingSubject) (string, string, error) {
	hostname := subject.Spec.Fqdn
	target := "http_status:404"

	// Generate cfHostname string from Subject Spec if not provided
	if hostname == "" {
		r.log.Info("Using current tunnel's domain for generating config")
		hostname = fmt.Sprintf("%s.%s", subject.Name, r.domain)
		r.log.Info("using default domain value", "domain", r.domain)
	}

	service := &corev1.Service{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: subject.Name, Namespace: r.binding.Namespace}, service); err != nil {
		r.log.Error(err, "Error getting referenced service")
		r.Recorder.Event(r.binding, corev1.EventTypeWarning, "FailedService", "Failed to get Service")
		return hostname, target, err
	}

	if len(service.Spec.Ports) == 0 {
		err := fmt.Errorf("no ports found in service spec, cannot proceed")
		r.log.Error(err, "unable to read service ports", "svc", service.Name)
		return hostname, target, err
	} else if len(service.Spec.Ports) > 1 {
		r.log.Info("Multiple ports definition found, picking the first in the list", "svc", service.Name)
	}

	servicePort := service.Spec.Ports[0]
	tunnelProto := subject.Spec.Protocol
	validProto := tunnelValidProtoMap[tunnelProto]

	serviceProto := r.getServiceProto(tunnelProto, validProto, servicePort)

	r.log.Info("Selected protocol", "protocol", serviceProto)

	target = fmt.Sprintf("%s://%s.%s.svc:%d", serviceProto, service.Name, service.Namespace, servicePort.Port)

	r.log.Info("generated cloudflare config", "hostname", hostname, "target", target)

	return hostname, target, nil
}

// getServiceProto returns the service protocol to be used
func (r *TunnelBindingReconciler) getServiceProto(tunnelProto string, validProto bool, servicePort corev1.ServicePort) string {
	var serviceProto string
	if tunnelProto != "" && !validProto {
		r.log.Info("Invalid Protocol provided, following default protocol logic")
	}

	if tunnelProto != "" && validProto {
		serviceProto = tunnelProto
	} else if servicePort.Protocol == corev1.ProtocolTCP {
		// Default protocol selection logic
		switch servicePort.Port {
		case 22:
			serviceProto = tunnelProtoSSH
		case 139, 445:
			serviceProto = tunnelProtoSMB
		case 443:
			serviceProto = tunnelProtoHTTPS
		case 3389:
			serviceProto = tunnelProtoRDP
		default:
			serviceProto = tunnelProtoHTTP
		}
	} else if servicePort.Protocol == corev1.ProtocolUDP {
		serviceProto = tunnelProtoUDP
	} else {
		err := fmt.Errorf("unsupported protocol")
		r.log.Error(err, "could not select protocol", "portProtocol", servicePort.Protocol, "annotationProtocol", tunnelProto)
	}
	return serviceProto
}

// configureCloudflareDaemon registers ingress rules to the CloudflareSyncState CRD
// via TunnelConfigService. The actual sync to Cloudflare API is handled by TunnelConfigSyncController.
//
// This is part of the unified sync architecture where:
// - TunnelBinding controller registers rules via TunnelConfigService
// - TunnelConfigSyncController aggregates all sources and syncs to Cloudflare API
func (r *TunnelBindingReconciler) configureCloudflareDaemon() error {
	bindings, err := r.getRelevantTunnelBindings()
	if err != nil {
		r.log.Error(err, "unable to get tunnel bindings")
		return err
	}

	// Build ingress rules from all bindings
	rules := make([]tunnelsvc.IngressRule, 0, 16)
	for _, binding := range bindings {
		for i, subject := range binding.Subjects {
			targetService := ""
			if subject.Spec.Target != "" {
				targetService = subject.Spec.Target
			} else {
				targetService = binding.Status.Services[i].Target
			}

			rule := tunnelsvc.IngressRule{
				Hostname: binding.Status.Services[i].Hostname,
				Service:  targetService,
				Path:     subject.Spec.Path,
			}

			// Convert origin request config
			originRequest := &tunnelsvc.OriginRequestConfig{}
			hasOriginConfig := false

			if subject.Spec.NoTlsVerify {
				originRequest.NoTLSVerify = &subject.Spec.NoTlsVerify
				hasOriginConfig = true
			}
			if subject.Spec.HTTP2Origin {
				originRequest.HTTP2Origin = &subject.Spec.HTTP2Origin
				hasOriginConfig = true
			}
			if subject.Spec.ProxyAddress != "" {
				originRequest.ProxyAddress = &subject.Spec.ProxyAddress
				hasOriginConfig = true
			}
			if subject.Spec.ProxyPort != 0 {
				originRequest.ProxyPort = &subject.Spec.ProxyPort
				hasOriginConfig = true
			}
			if subject.Spec.ProxyType != "" {
				originRequest.ProxyType = &subject.Spec.ProxyType
				hasOriginConfig = true
			}
			if caPool := subject.Spec.CaPool; caPool != "" {
				caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", caPool)
				originRequest.CAPool = &caPath
				hasOriginConfig = true
			}

			if hasOriginConfig {
				rule.OriginRequest = originRequest
			}

			rules = append(rules, rule)
		}
	}

	// Get credentials reference from tunnel
	credRef := r.getCredentialsReferenceFromTunnel()

	// Register rules to SyncState via TunnelConfigService
	svc := tunnelsvc.NewService(r.Client)
	opts := tunnelsvc.RegisterRulesOptions{
		TunnelID:       r.tunnelID,
		AccountID:      r.accountID,
		CredentialsRef: credRef,
		Source: service.Source{
			Kind:      "TunnelBinding",
			Namespace: r.binding.Namespace,
			Name:      r.binding.Name,
		},
		Rules:    rules,
		Priority: tunnelsvc.PriorityBinding,
	}

	if err := svc.RegisterRules(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register rules to SyncState", "tunnelID", r.tunnelID)
		return fmt.Errorf("register rules to SyncState: %w", err)
	}

	r.log.Info("Registered ingress rules to SyncState",
		"tunnelID", r.tunnelID,
		"ruleCount", len(rules),
		"source", fmt.Sprintf("TunnelBinding/%s/%s", r.binding.Namespace, r.binding.Name))
	return nil
}

// getCredentialsReferenceFromTunnel extracts the CredentialsReference from the referenced Tunnel or ClusterTunnel.
func (r *TunnelBindingReconciler) getCredentialsReferenceFromTunnel() networkingv1alpha2.CredentialsReference {
	kind := strings.ToLower(r.binding.TunnelRef.Kind)
	if kind == tunnelKindClusterTunnel {
		return r.getCredentialsFromClusterTunnel()
	}
	if kind == tunnelKindTunnel {
		return r.getCredentialsFromTunnel()
	}
	return networkingv1alpha2.CredentialsReference{}
}

// getCredentialsFromClusterTunnel extracts CredentialsReference from a ClusterTunnel.
func (r *TunnelBindingReconciler) getCredentialsFromClusterTunnel() networkingv1alpha2.CredentialsReference {
	clusterTunnel := &networkingv1alpha2.ClusterTunnel{}
	key := apitypes.NamespacedName{Name: r.binding.TunnelRef.Name, Namespace: r.Namespace}
	if err := r.Get(r.ctx, key, clusterTunnel); err != nil {
		return networkingv1alpha2.CredentialsReference{}
	}
	if clusterTunnel.Spec.Cloudflare.CredentialsRef == nil {
		return networkingv1alpha2.CredentialsReference{}
	}
	return networkingv1alpha2.CredentialsReference{
		Name: clusterTunnel.Spec.Cloudflare.CredentialsRef.Name,
	}
}

// getCredentialsFromTunnel extracts CredentialsReference from a Tunnel.
func (r *TunnelBindingReconciler) getCredentialsFromTunnel() networkingv1alpha2.CredentialsReference {
	tunnel := &networkingv1alpha2.Tunnel{}
	key := apitypes.NamespacedName{Name: r.binding.TunnelRef.Name, Namespace: r.binding.Namespace}
	if err := r.Get(r.ctx, key, tunnel); err != nil {
		return networkingv1alpha2.CredentialsReference{}
	}
	if tunnel.Spec.Cloudflare.CredentialsRef == nil {
		return networkingv1alpha2.CredentialsReference{}
	}
	return networkingv1alpha2.CredentialsReference{
		Name: tunnel.Spec.Cloudflare.CredentialsRef.Name,
	}
}

// findTunnelBindingsForTunnel returns reconcile requests for TunnelBindings that reference the given Tunnel
func (r *TunnelBindingReconciler) findTunnelBindingsForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all TunnelBindings
	bindings := &networkingv1alpha1.TunnelBindingList{}
	if err := r.List(ctx, bindings); err != nil {
		log.Error(err, "Failed to list TunnelBindings for Tunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, binding := range bindings.Items {
		if strings.ToLower(binding.TunnelRef.Kind) == tunnelKindTunnel &&
			binding.TunnelRef.Name == tunnel.Name &&
			binding.Namespace == tunnel.Namespace {
			log.Info("Tunnel changed, triggering TunnelBinding reconcile",
				"tunnel", tunnel.Name,
				"tunnelbinding", binding.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      binding.Name,
					Namespace: binding.Namespace,
				},
			})
		}
	}
	return requests
}

// findTunnelBindingsForClusterTunnel returns reconcile requests for TunnelBindings that reference the given ClusterTunnel
func (r *TunnelBindingReconciler) findTunnelBindingsForClusterTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	clusterTunnel, ok := obj.(*networkingv1alpha2.ClusterTunnel)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all TunnelBindings
	bindings := &networkingv1alpha1.TunnelBindingList{}
	if err := r.List(ctx, bindings); err != nil {
		log.Error(err, "Failed to list TunnelBindings for ClusterTunnel watch")
		return nil
	}

	var requests []reconcile.Request
	for _, binding := range bindings.Items {
		if strings.ToLower(binding.TunnelRef.Kind) == tunnelKindClusterTunnel &&
			binding.TunnelRef.Name == clusterTunnel.Name {
			log.Info("ClusterTunnel changed, triggering TunnelBinding reconcile",
				"clustertunnel", clusterTunnel.Name,
				"tunnelbinding", binding.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{
					Name:      binding.Name,
					Namespace: binding.Namespace,
				},
			})
		}
	}
	return requests
}

// findTunnelBindingsForService returns reconcile requests for TunnelBindings that reference the given Service
//
//nolint:revive // cognitive-complexity: watch handler logic is inherently complex
func (r *TunnelBindingReconciler) findTunnelBindingsForService(ctx context.Context, obj client.Object) []reconcile.Request {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}
	log := ctrllog.FromContext(ctx)

	// List all TunnelBindings in the same namespace
	bindings := &networkingv1alpha1.TunnelBindingList{}
	if err := r.List(ctx, bindings, client.InNamespace(svc.Namespace)); err != nil {
		log.Error(err, "Failed to list TunnelBindings for Service watch")
		return nil
	}

	var requests []reconcile.Request
	for _, binding := range bindings.Items {
		for _, subject := range binding.Subjects {
			if subject.Name == svc.Name {
				log.Info("Service changed, triggering TunnelBinding reconcile",
					"service", svc.Name,
					"tunnelbinding", binding.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: apitypes.NamespacedName{
						Name:      binding.Name,
						Namespace: binding.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *TunnelBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("cloudflare-operator")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha1.TunnelBinding{}).
		// P0 FIX: Watch Tunnel changes to trigger TunnelBinding reconcile when credentials change
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findTunnelBindingsForTunnel),
		).
		// P0 FIX: Watch ClusterTunnel changes to trigger TunnelBinding reconcile
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findTunnelBindingsForClusterTunnel),
		).
		// P1 FIX: Watch Service changes to trigger TunnelBinding reconcile when ports change
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findTunnelBindingsForService),
		).
		Complete(r)
}
