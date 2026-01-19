// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package accessapplication provides the L2 controller for AccessApplication CRD.
// This controller handles the reconciliation of AccessApplication resources,
// resolving policy references and registering configurations with the Core Service.
package accessapplication

import (
	"context"
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
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
)

const (
	FinalizerName = "cloudflare.com/accessapplication-finalizer"
	// StateActive indicates the resource is actively synced with Cloudflare
	StateActive = "active"
)

// Reconciler reconciles an AccessApplication object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Service layer
	appService *accesssvc.ApplicationService

	// Runtime state
	ctx context.Context
	log logr.Logger
	app *networkingv1alpha2.AccessApplication
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop for AccessApplication resources.
//
//nolint:revive // cognitive complexity is acceptable for this controller reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	// Fetch the AccessApplication instance
	r.app = &networkingv1alpha2.AccessApplication{}
	if err := r.Get(ctx, req.NamespacedName, r.app); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("AccessApplication deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch AccessApplication")
		return ctrl.Result{}, err
	}

	// Check if AccessApplication is being deleted
	// Following Unified Sync Architecture: only unregister from SyncState
	// Sync Controller handles actual Cloudflare API deletion
	if r.app.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.app, FinalizerName) {
		controllerutil.AddFinalizer(r.app, FinalizerName)
		if err := r.Update(ctx, r.app); err != nil {
			r.log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonFinalizerSet, "Finalizer added")
	}

	// Reconcile the AccessApplication through service layer
	result, err := r.reconcileApplication()
	if err != nil {
		r.log.Error(err, "failed to reconcile AccessApplication")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return result, nil
}

// resolveCredentials resolves the credentials reference and returns the credentials info.
// Following Unified Sync Architecture, the Resource Controller only needs
// credential metadata (accountID, credRef) - it does not create a Cloudflare API client.
func (r *Reconciler) resolveCredentials() (*controller.CredentialsInfo, error) {
	// AccessApplication is cluster-scoped, use operator namespace for legacy inline secrets
	info, err := controller.ResolveCredentialsForService(
		r.ctx,
		r.Client,
		r.log,
		r.app.Spec.Cloudflare,
		controller.OperatorNamespace,
		r.app.Status.AccountID,
	)
	if err != nil {
		r.log.Error(err, "failed to resolve credentials")
		r.Recorder.Event(r.app, corev1.EventTypeWarning, controller.EventReasonAPIError,
			"Failed to resolve credentials: "+err.Error())
		return nil, err
	}

	return info, nil
}

// handleDeletion handles the deletion of an AccessApplication.
// Following Unified Sync Architecture:
// Resource Controller only unregisters from SyncState.
// AccessApplication Sync Controller handles the actual Cloudflare API deletion.
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.app, FinalizerName) {
		return ctrl.Result{}, nil
	}

	r.log.Info("Unregistering AccessApplication from SyncState")

	// Get Application ID from status
	applicationID := r.app.Status.ApplicationID

	// Unregister from SyncState - this triggers Sync Controller to delete from Cloudflare
	// Following: Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
	source := service.Source{
		Kind: "AccessApplication",
		Name: r.app.Name,
	}

	if err := r.appService.Unregister(r.ctx, applicationID, source); err != nil {
		r.log.Error(err, "failed to unregister from SyncState")
		r.Recorder.Event(r.app, corev1.EventTypeWarning, "UnregisterFailed",
			fmt.Sprintf("Failed to unregister from SyncState: %s", err.Error()))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	r.Recorder.Event(r.app, corev1.EventTypeNormal, "Unregistered",
		"Unregistered from SyncState, Sync Controller will delete from Cloudflare")

	// Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.app, func() {
		controllerutil.RemoveFinalizer(r.app, FinalizerName)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return ctrl.Result{}, nil
}

// reconcileApplication ensures the AccessApplication configuration is registered with the service layer.
// Following Unified Sync Architecture:
// Resource Controller → Core Service → SyncState → Sync Controller → Cloudflare API
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation logic
func (r *Reconciler) reconcileApplication() (ctrl.Result, error) {
	appName := r.app.GetAccessApplicationName()

	// Resolve credentials (without creating API client)
	credInfo, err := r.resolveCredentials()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve credentials: %w", err)
	}

	// Resolve IdP references
	allowedIdps := make([]string, 0, len(r.app.Spec.AllowedIdps)+len(r.app.Spec.AllowedIdpRefs))
	allowedIdps = append(allowedIdps, r.app.Spec.AllowedIdps...)
	for _, ref := range r.app.Spec.AllowedIdpRefs {
		idp := &networkingv1alpha2.AccessIdentityProvider{}
		if err := r.Get(r.ctx, apitypes.NamespacedName{Name: ref.Name}, idp); err != nil {
			r.log.Error(err, "failed to get AccessIdentityProvider", "name", ref.Name)
			continue
		}
		if idp.Status.ProviderID != "" {
			allowedIdps = append(allowedIdps, idp.Status.ProviderID)
		}
	}

	// Resolve policy references
	policies, err := r.resolvePolicies()
	if err != nil {
		r.log.Error(err, "failed to resolve policies")
		// Continue with empty policies, they will be retried
	}

	// Build the configuration
	config := accesssvc.AccessApplicationConfig{
		Name:                     appName,
		Domain:                   r.app.Spec.Domain,
		SelfHostedDomains:        r.app.Spec.SelfHostedDomains,
		Destinations:             r.app.Spec.Destinations,
		DomainType:               r.app.Spec.DomainType,
		PrivateAddress:           r.app.Spec.PrivateAddress,
		Type:                     r.app.Spec.Type,
		SessionDuration:          r.app.Spec.SessionDuration,
		AllowedIdps:              allowedIdps,
		AutoRedirectToIdentity:   r.app.Spec.AutoRedirectToIdentity,
		EnableBindingCookie:      r.app.Spec.EnableBindingCookie,
		HTTPOnlyCookieAttribute:  r.app.Spec.HttpOnlyCookieAttribute,
		PathCookieAttribute:      r.app.Spec.PathCookieAttribute,
		SameSiteCookieAttribute:  r.app.Spec.SameSiteCookieAttribute,
		LogoURL:                  r.app.Spec.LogoURL,
		SkipInterstitial:         r.app.Spec.SkipInterstitial,
		OptionsPreflightBypass:   r.app.Spec.OptionsPreflightBypass,
		AppLauncherVisible:       r.app.Spec.AppLauncherVisible,
		ServiceAuth401Redirect:   r.app.Spec.ServiceAuth401Redirect,
		CustomDenyMessage:        r.app.Spec.CustomDenyMessage,
		CustomDenyURL:            r.app.Spec.CustomDenyURL,
		CustomNonIdentityDenyURL: r.app.Spec.CustomNonIdentityDenyURL,
		AllowAuthenticateViaWarp: r.app.Spec.AllowAuthenticateViaWarp,
		Tags:                     r.app.Spec.Tags,
		CustomPages:              r.app.Spec.CustomPages,
		GatewayRules:             r.app.Spec.GatewayRules,
		CorsHeaders:              r.app.Spec.CorsHeaders,
		SaasApp:                  r.app.Spec.SaasApp,
		SCIMConfig:               r.app.Spec.SCIMConfig,
		AppLauncherCustomization: r.app.Spec.AppLauncherCustomization,
		TargetContexts:           r.app.Spec.TargetContexts,
		Policies:                 policies,
	}

	// Build source reference
	source := service.Source{
		Kind: "AccessApplication",
		Name: r.app.Name,
	}

	// Register with service using credentials info
	opts := accesssvc.AccessApplicationRegisterOptions{
		AccountID:      credInfo.AccountID,
		ApplicationID:  r.app.Status.ApplicationID,
		Source:         source,
		Config:         config,
		CredentialsRef: credInfo.CredentialsRef,
	}

	if err := r.appService.Register(r.ctx, opts); err != nil {
		r.log.Error(err, "failed to register AccessApplication configuration")
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		r.Recorder.Event(r.app, corev1.EventTypeWarning, controller.EventReasonCreateFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Check if already synced (SyncState may have been created and synced in a previous reconcile)
	syncStatus, err := r.appService.GetSyncStatus(r.ctx, source, r.app.Status.ApplicationID)
	if err != nil {
		r.log.Error(err, "failed to get sync status")
		if err := r.updateStatusPending(credInfo.AccountID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if syncStatus != nil && syncStatus.IsSynced && syncStatus.ApplicationID != "" {
		// Already synced, update status to Ready
		if err := r.updateStatusReady(credInfo.AccountID, syncStatus.ApplicationID); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Update status to Pending if not already synced
	if err := r.updateStatusPending(credInfo.AccountID); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// resolvePolicies converts policy references to AccessPolicyConfig.
// Following Unified Sync Architecture: L2 Controller passes reference info to L3 Service,
// and L5 Sync Controller resolves actual Group IDs via Cloudflare API.
//
// Supports two modes:
// 1. Inline Rules Mode: When include/exclude/require rules are specified directly
// 2. Group Reference Mode: When referencing an AccessGroup via name/groupId/cloudflareGroupName
//
//nolint:revive,unparam // cognitive-complexity: Policy resolution inherently handles multiple modes and error paths; error kept for future use
func (r *Reconciler) resolvePolicies() ([]accesssvc.AccessPolicyConfig, error) {
	if len(r.app.Spec.Policies) == 0 {
		return nil, nil
	}

	policies := make([]accesssvc.AccessPolicyConfig, 0, len(r.app.Spec.Policies))

	for i, policyRef := range r.app.Spec.Policies {
		precedence := policyRef.Precedence
		if precedence == 0 {
			precedence = i + 1 // Auto-assign precedence based on order
		}

		decision := policyRef.Decision
		if decision == "" {
			decision = "allow"
		}

		policyConfig := accesssvc.AccessPolicyConfig{
			Decision:        decision,
			Precedence:      precedence,
			PolicyName:      policyRef.PolicyName,
			SessionDuration: policyRef.SessionDuration,
		}

		// Check if using inline rules mode (include/exclude/require specified directly)
		hasInlineRules := len(policyRef.Include) > 0 || len(policyRef.Exclude) > 0 || len(policyRef.Require) > 0

		if hasInlineRules {
			// Inline Rules Mode - pass rules directly to L5 Sync Controller
			policyConfig.Include = policyRef.Include
			policyConfig.Exclude = policyRef.Exclude
			policyConfig.Require = policyRef.Require

			r.log.V(1).Info("Using inline rules mode for policy",
				"policyIndex", i, "precedence", precedence,
				"includeCount", len(policyRef.Include),
				"excludeCount", len(policyRef.Exclude),
				"requireCount", len(policyRef.Require))
		} else {
			// Group Reference Mode - set reference fields for L5 to resolve
			switch {
			case policyRef.GroupID != "":
				// Direct Cloudflare group ID reference - will be validated in L5
				policyConfig.CloudflareGroupID = policyRef.GroupID
			case policyRef.CloudflareGroupName != "":
				// Cloudflare group name reference - will be looked up in L5
				policyConfig.CloudflareGroupName = policyRef.CloudflareGroupName
			case policyRef.Name != "":
				// K8s AccessGroup reference - resolve now to get K8s resource status
				accessGroup := &networkingv1alpha2.AccessGroup{}
				if err := r.Get(r.ctx, apitypes.NamespacedName{Name: policyRef.Name}, accessGroup); err != nil {
					if apierrors.IsNotFound(err) {
						r.log.Error(err, "Kubernetes AccessGroup resource not found",
							"policyIndex", i, "accessGroupName", policyRef.Name)
						r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyResolutionFailed",
							fmt.Sprintf("AccessGroup %s not found for policy at precedence %d", policyRef.Name, precedence))
						continue
					}
					r.log.Error(err, "Failed to get AccessGroup resource",
						"policyIndex", i, "accessGroupName", policyRef.Name)
					continue
				}
				if accessGroup.Status.GroupID != "" {
					// AccessGroup already synced - use its GroupID directly
					policyConfig.GroupID = accessGroup.Status.GroupID
					policyConfig.GroupName = accessGroup.GetAccessGroupName()
				} else {
					// AccessGroup not yet synced - pass reference name for L5 to resolve
					policyConfig.K8sAccessGroupName = policyRef.Name
				}
			default:
				r.log.Info("No group reference or inline rules specified in policy, skipping",
					"policyIndex", i, "precedence", precedence)
				r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyResolutionFailed",
					fmt.Sprintf("No group reference or inline rules specified for policy at precedence %d", precedence))
				continue
			}
		}

		policies = append(policies, policyConfig)
	}

	return policies, nil
}

// updateStatusPending updates the AccessApplication status to Pending state.
func (r *Reconciler) updateStatusPending(accountID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.app, func() {
		r.app.Status.ObservedGeneration = r.app.Generation

		// Keep existing state if already active, otherwise set to pending
		if r.app.Status.State != StateActive {
			r.app.Status.State = "pending"
		}

		// Set account ID
		if accountID != "" {
			r.app.Status.AccountID = accountID
		}

		r.setCondition(metav1.ConditionFalse, "Pending", "Configuration registered, waiting for sync")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessApplication status")
		return err
	}

	r.log.Info("AccessApplication configuration registered", "name", r.app.Name)
	r.Recorder.Event(r.app, corev1.EventTypeNormal, "Registered",
		"Configuration registered to SyncState")
	return nil
}

// updateStatusReady updates the AccessApplication status to Ready state.
func (r *Reconciler) updateStatusReady(accountID, applicationID string) error {
	err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.app, func() {
		r.app.Status.ObservedGeneration = r.app.Generation
		r.app.Status.State = StateActive
		r.app.Status.AccountID = accountID
		r.app.Status.ApplicationID = applicationID
		r.setCondition(metav1.ConditionTrue, "Synced", "AccessApplication synced to Cloudflare")
	})

	if err != nil {
		r.log.Error(err, "failed to update AccessApplication status to Ready")
		return err
	}

	r.log.Info("AccessApplication synced successfully", "name", r.app.Name, "applicationId", applicationID)
	r.Recorder.Event(r.app, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("AccessApplication synced to Cloudflare with ID %s", applicationID))
	return nil
}

// setCondition sets a condition on the AccessApplication status.
func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&r.app.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.app.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// ============================================================================
// Watch handler helper functions
// ============================================================================

// appReferencesIDP checks if an AccessApplication references the given AccessIdentityProvider.
func appReferencesIDP(app *networkingv1alpha2.AccessApplication, idpName string) bool {
	for _, ref := range app.Spec.AllowedIdpRefs {
		if ref.Name == idpName {
			return true
		}
	}
	return false
}

// appReferencesAccessGroup checks if an AccessApplication references the given AccessGroup
// via the K8s resource name field in policies.
func appReferencesAccessGroup(app *networkingv1alpha2.AccessApplication, groupName string) bool {
	for _, policy := range app.Spec.Policies {
		if policy.Name == groupName {
			return true
		}
	}
	return false
}

// findAccessApplicationsForIdentityProvider returns a list of reconcile requests for AccessApplications
// that reference the given AccessIdentityProvider.
func (r *Reconciler) findAccessApplicationsForIdentityProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	idp, ok := obj.(*networkingv1alpha2.AccessIdentityProvider)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for IdentityProvider watch")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(appList.Items))
	for i := range appList.Items {
		app := &appList.Items[i]
		if !appReferencesIDP(app, idp.Name) {
			continue
		}

		logger.Info("AccessIdentityProvider changed, triggering AccessApplication reconcile",
			"identityprovider", idp.Name,
			"accessapplication", app.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: apitypes.NamespacedName{Name: app.Name},
		})
	}

	return requests
}

// findAccessApplicationsForAccessGroup returns a list of reconcile requests for AccessApplications
// that reference the given AccessGroup via the name field in policies.
func (r *Reconciler) findAccessApplicationsForAccessGroup(ctx context.Context, obj client.Object) []reconcile.Request {
	accessGroup, ok := obj.(*networkingv1alpha2.AccessGroup)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for AccessGroup watch")
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range appList.Items {
		app := &appList.Items[i]
		if !appReferencesAccessGroup(app, accessGroup.Name) {
			continue
		}

		logger.Info("AccessGroup changed, triggering AccessApplication reconcile",
			"accessgroup", accessGroup.Name,
			"accessapplication", app.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: apitypes.NamespacedName{Name: app.Name},
		})
	}

	return requests
}

// findAccessApplicationsForIngress returns reconcile requests for AccessApplications
// whose domains match the Ingress hosts.
func (r *Reconciler) findAccessApplicationsForIngress(ctx context.Context, obj client.Object) []reconcile.Request {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	hosts := make([]string, 0)
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	if len(hosts) == 0 {
		return nil
	}

	return r.findAccessApplicationsMatchingDomains(ctx, logger, hosts, "Ingress", ing.Name)
}

// findAccessApplicationsForClusterTunnel returns reconcile requests for AccessApplications
// when a ClusterTunnel changes.
func (r *Reconciler) findAccessApplicationsForClusterTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.ClusterTunnel)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	return r.findPendingAccessApplications(ctx, logger, "ClusterTunnel", tunnel.Name)
}

// findAccessApplicationsForTunnel returns reconcile requests for AccessApplications
// when a Tunnel changes.
func (r *Reconciler) findAccessApplicationsForTunnel(ctx context.Context, obj client.Object) []reconcile.Request {
	tunnel, ok := obj.(*networkingv1alpha2.Tunnel)
	if !ok {
		return nil
	}
	logger := ctrllog.FromContext(ctx)

	return r.findPendingAccessApplications(ctx, logger, "Tunnel", tunnel.Name)
}

// findAccessApplicationsMatchingDomains finds AccessApplications whose domain
// matches any of the given hosts.
func (r *Reconciler) findAccessApplicationsMatchingDomains(
	ctx context.Context,
	logger logr.Logger,
	hosts []string,
	sourceType, sourceName string,
) []reconcile.Request {
	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for domain matching")
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range appList.Items {
		app := &appList.Items[i]

		// Only trigger if the app is in a non-ready state
		if app.Status.State == StateActive {
			continue
		}

		if domainMatchesHosts(app.Spec.Domain, hosts) ||
			anyDomainMatchesHosts(app.Spec.SelfHostedDomains, hosts) {
			logger.Info("Domain match found, triggering AccessApplication reconcile",
				"sourceType", sourceType,
				"sourceName", sourceName,
				"accessapplication", app.Name,
				"domain", app.Spec.Domain)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name},
			})
		}
	}

	return requests
}

// findPendingAccessApplications finds AccessApplications that are not in ready state.
func (r *Reconciler) findPendingAccessApplications(
	ctx context.Context,
	logger logr.Logger,
	sourceType, sourceName string,
) []reconcile.Request {
	appList := &networkingv1alpha2.AccessApplicationList{}
	if err := r.List(ctx, appList); err != nil {
		logger.Error(err, "Failed to list AccessApplications for tunnel watch")
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range appList.Items {
		app := &appList.Items[i]

		// Only trigger if the app is in a non-ready state
		if app.Status.State == StateActive {
			continue
		}

		readyCondition := meta.FindStatusCondition(app.Status.Conditions, "Ready")
		if readyCondition != nil && readyCondition.Status == metav1.ConditionFalse {
			logger.Info("Tunnel changed, triggering pending AccessApplication reconcile",
				"sourceType", sourceType,
				"sourceName", sourceName,
				"accessapplication", app.Name,
				"currentState", app.Status.State,
				"reason", readyCondition.Reason)
			requests = append(requests, reconcile.Request{
				NamespacedName: apitypes.NamespacedName{Name: app.Name},
			})
		}
	}

	return requests
}

// domainMatchesHosts checks if a domain matches any of the given hosts.
// DNS domain names are case-insensitive, so comparisons are done in lowercase.
//
//nolint:revive // cognitive complexity is acceptable for this domain matching logic
func domainMatchesHosts(domain string, hosts []string) bool {
	if domain == "" {
		return false
	}

	// Normalize domain to lowercase for case-insensitive comparison
	domainLower := strings.ToLower(domain)

	for _, host := range hosts {
		// Normalize host to lowercase for case-insensitive comparison
		hostLower := strings.ToLower(host)

		if domainLower == hostLower {
			return true
		}
		// Check wildcard matching
		if len(hostLower) > 2 && hostLower[:2] == "*." {
			baseDomain := hostLower[2:]
			if len(domainLower) > len(baseDomain)+1 && domainLower[len(domainLower)-len(baseDomain)-1:] == "."+baseDomain {
				return true
			}
			if domainLower == baseDomain {
				return true
			}
		}
		if len(domainLower) > 2 && domainLower[:2] == "*." {
			baseDomain := domainLower[2:]
			if len(hostLower) > len(baseDomain)+1 && hostLower[len(hostLower)-len(baseDomain)-1:] == "."+baseDomain {
				return true
			}
			if hostLower == baseDomain {
				return true
			}
		}
	}
	return false
}

// anyDomainMatchesHosts checks if any domain in the list matches any host.
func anyDomainMatchesHosts(domains []string, hosts []string) bool {
	for _, domain := range domains {
		if domainMatchesHosts(domain, hosts) {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessapplication-controller")
	r.appService = accesssvc.NewApplicationService(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessApplication{}).
		// Watch AccessIdentityProvider changes for IdP reference updates
		Watches(
			&networkingv1alpha2.AccessIdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForIdentityProvider),
		).
		// Watch AccessGroup changes for policy reference updates
		Watches(
			&networkingv1alpha2.AccessGroup{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForAccessGroup),
		).
		// Watch Ingress changes - when an Ingress is created/updated, its domains
		// become available in the tunnel, allowing AccessApplications to be created
		Watches(
			&networkingv1.Ingress{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForIngress),
		).
		// Watch ClusterTunnel changes - tunnel config changes affect domain availability
		Watches(
			&networkingv1alpha2.ClusterTunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForClusterTunnel),
		).
		// Watch Tunnel changes - tunnel config changes affect domain availability
		Watches(
			&networkingv1alpha2.Tunnel{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessApplicationsForTunnel),
		).
		Complete(r)
}
