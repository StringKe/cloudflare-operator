// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessapplication

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/controller"
)

const (
	AccessApplicationFinalizer = "cloudflare.com/accessapplication-finalizer"
)

// Reconciler reconciles an AccessApplication object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ctx   context.Context
	log   logr.Logger
	app   *networkingv1alpha2.AccessApplication
	cfAPI *cf.API
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=accessapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrllog.FromContext(ctx)

	r.app = &networkingv1alpha2.AccessApplication{}
	if err := r.Get(ctx, req.NamespacedName, r.app); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.initAPIClient(); err != nil {
		r.setCondition(metav1.ConditionFalse, controller.EventReasonAPIError, err.Error())
		return ctrl.Result{}, err
	}

	if r.app.GetDeletionTimestamp() != nil {
		return r.handleDeletion()
	}

	if !controllerutil.ContainsFinalizer(r.app, AccessApplicationFinalizer) {
		controllerutil.AddFinalizer(r.app, AccessApplicationFinalizer)
		if err := r.Update(ctx, r.app); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileApplication(); err != nil {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) initAPIClient() error {
	// AccessApplication is cluster-scoped, use operator namespace for legacy inline secrets
	api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client, controller.OperatorNamespace, r.app.Spec.Cloudflare)
	if err != nil {
		r.log.Error(err, "failed to initialize API client")
		return err
	}

	// Preserve validated account ID from status
	api.ValidAccountId = r.app.Status.AccountID
	r.cfAPI = api

	return nil
}

func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.app, AccessApplicationFinalizer) {
		return ctrl.Result{}, nil
	}

	if r.app.Status.ApplicationID != "" {
		if err := r.cfAPI.DeleteAccessApplication(r.app.Status.ApplicationID); err != nil {
			// P0 FIX: Check if resource is already deleted (NotFound error)
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete AccessApplication from Cloudflare")
				r.Recorder.Event(r.app, corev1.EventTypeWarning, controller.EventReasonDeleteFailed, cf.SanitizeErrorMessage(err))
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			r.log.Info("AccessApplication already deleted from Cloudflare", "id", r.app.Status.ApplicationID)
			r.Recorder.Event(r.app, corev1.EventTypeNormal, "AlreadyDeleted", "AccessApplication was already deleted from Cloudflare")
		} else {
			r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
		}
	}

	// P0 FIX: Remove finalizer with retry logic to handle conflicts
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.app, func() {
		controllerutil.RemoveFinalizer(r.app, AccessApplicationFinalizer)
	}); err != nil {
		r.log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileApplication() error {
	appName := r.app.GetAccessApplicationName()

	// Resolve IdP references
	allowedIdps := r.app.Spec.AllowedIdps
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

	params := cf.AccessApplicationParams{
		Name:                     appName,
		Domain:                   r.app.Spec.Domain,
		Type:                     r.app.Spec.Type,
		SessionDuration:          r.app.Spec.SessionDuration,
		AllowedIdps:              allowedIdps,
		AutoRedirectToIdentity:   &r.app.Spec.AutoRedirectToIdentity,
		EnableBindingCookie:      r.app.Spec.EnableBindingCookie,
		HttpOnlyCookieAttribute:  r.app.Spec.HttpOnlyCookieAttribute,
		SameSiteCookieAttribute:  r.app.Spec.SameSiteCookieAttribute,
		LogoURL:                  r.app.Spec.LogoURL,
		SkipInterstitial:         r.app.Spec.SkipInterstitial,
		AppLauncherVisible:       r.app.Spec.AppLauncherVisible,
		ServiceAuth401Redirect:   r.app.Spec.ServiceAuth401Redirect,
		CustomDenyMessage:        r.app.Spec.CustomDenyMessage,
		CustomDenyURL:            r.app.Spec.CustomDenyURL,
		AllowAuthenticateViaWarp: r.app.Spec.AllowAuthenticateViaWarp,
		Tags:                     r.app.Spec.Tags,
	}

	if r.app.Status.ApplicationID != "" {
		return r.updateApplication(params)
	}

	// Try to find existing
	existing, err := r.cfAPI.ListAccessApplicationsByName(appName)
	if err == nil && existing != nil {
		r.log.Info("Found existing AccessApplication, adopting", "id", existing.ID)

		// Adopt the application and reconcile policies
		if err := r.updateStatus(existing, nil); err != nil {
			return err
		}

		// Reconcile policies for adopted application
		resolvedPolicies, policyErr := r.reconcilePolicies(existing.ID)
		if policyErr != nil {
			r.log.Error(policyErr, "failed to reconcile policies after adopting application")
			r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyReconcileFailed",
				fmt.Sprintf("Policy reconciliation failed: %s", cf.SanitizeErrorMessage(policyErr)))
		}

		return r.updateStatus(existing, resolvedPolicies)
	}

	return r.createApplication(params)
}

func (r *Reconciler) createApplication(params cf.AccessApplicationParams) error {
	r.Recorder.Event(r.app, corev1.EventTypeNormal, "Creating", "Creating AccessApplication")

	result, err := r.cfAPI.CreateAccessApplication(params)
	if err != nil {
		r.setCondition(metav1.ConditionFalse, controller.EventReasonCreateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonCreated, "Created AccessApplication")

	// Update status first to get ApplicationID
	if err := r.updateStatus(result, nil); err != nil {
		return err
	}

	// Now reconcile policies
	resolvedPolicies, err := r.reconcilePolicies(result.ID)
	if err != nil {
		r.log.Error(err, "failed to reconcile policies after creating application")
		r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyReconcileFailed",
			fmt.Sprintf("Policy reconciliation failed: %s", cf.SanitizeErrorMessage(err)))
		// Don't fail the entire reconcile, policies can be retried
	}

	// Update status again with policy information
	return r.updateStatus(result, resolvedPolicies)
}

func (r *Reconciler) updateApplication(params cf.AccessApplicationParams) error {
	result, err := r.cfAPI.UpdateAccessApplication(r.app.Status.ApplicationID, params)
	if err != nil {
		r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonUpdated, "Updated AccessApplication")

	// Reconcile policies
	resolvedPolicies, err := r.reconcilePolicies(r.app.Status.ApplicationID)
	if err != nil {
		r.log.Error(err, "failed to reconcile policies after updating application")
		r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyReconcileFailed",
			fmt.Sprintf("Policy reconciliation failed: %s", cf.SanitizeErrorMessage(err)))
		// Don't fail the entire reconcile, policies can be retried
	}

	return r.updateStatus(result, resolvedPolicies)
}

func (r *Reconciler) updateStatus(result *cf.AccessApplicationResult, resolvedPolicies []networkingv1alpha2.ResolvedPolicyStatus) error {
	// Use retry logic for status updates to handle conflicts
	return controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.app, func() {
		r.app.Status.ApplicationID = result.ID
		r.app.Status.AUD = result.AUD
		r.app.Status.AccountID = r.cfAPI.ValidAccountId
		r.app.Status.Domain = result.Domain
		r.app.Status.State = "active"
		r.app.Status.ObservedGeneration = r.app.Generation
		r.app.Status.ResolvedPolicies = resolvedPolicies

		r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "Reconciled successfully")
	})
}

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

// resolvedPolicy contains the resolved information for a single policy.
type resolvedPolicy struct {
	ref       networkingv1alpha2.AccessPolicyRef
	groupID   string
	groupName string
	source    string // k8s, groupId, cloudflareGroupName
}

// getPolicyName returns the policy name for Cloudflare.
func (p *resolvedPolicy) getPolicyName(appName string, precedence int) string {
	if p.ref.PolicyName != "" {
		return p.ref.PolicyName
	}
	return fmt.Sprintf("%s-policy-%d", appName, precedence)
}

// ErrNoGroupReference is returned when no group reference is specified in a policy.
var ErrNoGroupReference = errors.New("no group reference specified in policy (must specify one of: name, groupId, cloudflareGroupName)")

// resolveAccessGroupID resolves the group ID from various reference types.
// Priority: groupId > cloudflareGroupName > name
// Returns: (groupID, groupName, source, error)
//
//nolint:revive // cognitive complexity is acceptable for this decision tree
func (r *Reconciler) resolveAccessGroupID(ref networkingv1alpha2.AccessPolicyRef) (string, string, string, error) {
	switch {
	case ref.GroupID != "":
		return r.resolveByGroupID(ref.GroupID)
	case ref.CloudflareGroupName != "":
		return r.resolveByCloudflareGroupName(ref.CloudflareGroupName)
	case ref.Name != "":
		return r.resolveByK8sAccessGroup(ref.Name)
	default:
		return "", "", "", ErrNoGroupReference
	}
}

// resolveByGroupID validates and resolves a direct Cloudflare group ID.
//
//nolint:revive // 4 return values are needed to match resolveAccessGroupID signature
func (r *Reconciler) resolveByGroupID(groupID string) (resolvedGroupID, resolvedGroupName, source string, err error) {
	group, err := r.cfAPI.GetAccessGroup(groupID)
	if err != nil {
		if cf.IsNotFoundError(err) {
			return "", "", "", fmt.Errorf("cloudflare access group not found: %s", groupID)
		}
		return "", "", "", fmt.Errorf("failed to validate cloudflare group: %w", err)
	}
	return groupID, group.Name, "groupId", nil
}

// resolveByCloudflareGroupName looks up a Cloudflare group by its display name.
//
//nolint:revive // 4 return values are needed to match resolveAccessGroupID signature
func (r *Reconciler) resolveByCloudflareGroupName(groupName string) (resolvedGroupID, resolvedGroupName, source string, err error) {
	group, err := r.cfAPI.ListAccessGroupsByName(groupName)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to lookup cloudflare group by name: %w", err)
	}
	if group == nil {
		return "", "", "", fmt.Errorf("cloudflare access group not found by name: %s", groupName)
	}
	return group.ID, group.Name, "cloudflareGroupName", nil
}

// resolveByK8sAccessGroup looks up a Kubernetes AccessGroup resource.
//
//nolint:revive // 4 return values are needed to match resolveAccessGroupID signature
func (r *Reconciler) resolveByK8sAccessGroup(name string) (resolvedGroupID, resolvedGroupName, source string, err error) {
	accessGroup := &networkingv1alpha2.AccessGroup{}
	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: name}, accessGroup); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", "", fmt.Errorf("kubernetes AccessGroup resource not found: %s", name)
		}
		return "", "", "", fmt.Errorf("failed to get AccessGroup resource: %w", err)
	}
	if accessGroup.Status.GroupID == "" {
		return "", "", "", fmt.Errorf("AccessGroup %s not yet ready (no GroupID in status)", name)
	}
	return accessGroup.Status.GroupID, accessGroup.GetAccessGroupName(), "k8s", nil
}

// reconcilePolicies manages the Access Policies for an application.
//
//nolint:revive // cognitive complexity is acceptable for this reconciliation logic
func (r *Reconciler) reconcilePolicies(applicationID string) ([]networkingv1alpha2.ResolvedPolicyStatus, error) {
	if len(r.app.Spec.Policies) == 0 {
		// No policies specified, clean up any existing policies
		if err := r.cleanupAllPolicies(applicationID); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Resolve all policies
	desiredPolicies := make(map[int]resolvedPolicy)
	resolvedStatuses := make([]networkingv1alpha2.ResolvedPolicyStatus, 0, len(r.app.Spec.Policies))

	for i, policyRef := range r.app.Spec.Policies {
		precedence := policyRef.Precedence
		if precedence == 0 {
			precedence = i + 1 // Auto-assign precedence based on order
		}

		groupID, groupName, source, err := r.resolveAccessGroupID(policyRef)
		if err != nil {
			r.log.Error(err, "failed to resolve access group",
				"policyIndex", i, "precedence", precedence)
			r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyResolutionFailed",
				fmt.Sprintf("Failed to resolve policy at precedence %d: %s", precedence, cf.SanitizeErrorMessage(err)))
			// Continue with other policies but record this failure
			continue
		}

		desiredPolicies[precedence] = resolvedPolicy{
			ref:       policyRef,
			groupID:   groupID,
			groupName: groupName,
			source:    source,
		}
	}

	// Sync policies to Cloudflare
	policyStatuses, err := r.syncPolicies(applicationID, desiredPolicies)
	if err != nil {
		return nil, err
	}

	resolvedStatuses = append(resolvedStatuses, policyStatuses...)

	// Sort by precedence for consistent status
	sort.Slice(resolvedStatuses, func(i, j int) bool {
		return resolvedStatuses[i].Precedence < resolvedStatuses[j].Precedence
	})

	return resolvedStatuses, nil
}

// syncPolicies ensures the Cloudflare policies match the desired state.
//
//nolint:revive // cognitive complexity is acceptable for this sync logic
func (r *Reconciler) syncPolicies(applicationID string, desired map[int]resolvedPolicy) ([]networkingv1alpha2.ResolvedPolicyStatus, error) {
	// Get existing policies
	existing, err := r.cfAPI.ListAccessPolicies(applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing policies: %w", err)
	}

	existingMap := make(map[int]*cf.AccessPolicyResult)
	for i := range existing {
		existingMap[existing[i].Precedence] = &existing[i]
	}

	resolvedStatuses := make([]networkingv1alpha2.ResolvedPolicyStatus, 0, len(desired))

	// Create or update policies
	for precedence, policy := range desired {
		decision := policy.ref.Decision
		if decision == "" {
			decision = "allow"
		}

		params := cf.AccessPolicyParams{
			ApplicationID: applicationID,
			Name:          policy.getPolicyName(r.app.GetAccessApplicationName(), precedence),
			Decision:      decision,
			Precedence:    precedence,
			Include:       []interface{}{cf.BuildGroupIncludeRule(policy.groupID)},
		}

		if policy.ref.SessionDuration != "" {
			params.SessionDuration = &policy.ref.SessionDuration
		}

		var policyID string
		if existingPolicy, ok := existingMap[precedence]; ok {
			// Update existing policy
			result, err := r.cfAPI.UpdateAccessPolicy(existingPolicy.ID, params)
			if err != nil {
				r.log.Error(err, "failed to update policy", "precedence", precedence)
				r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyUpdateFailed",
					fmt.Sprintf("Failed to update policy at precedence %d: %s", precedence, cf.SanitizeErrorMessage(err)))
				continue
			}
			policyID = result.ID
			delete(existingMap, precedence) // Mark as processed
		} else {
			// Create new policy
			result, err := r.cfAPI.CreateAccessPolicy(params)
			if err != nil {
				r.log.Error(err, "failed to create policy", "precedence", precedence)
				r.Recorder.Event(r.app, corev1.EventTypeWarning, "PolicyCreateFailed",
					fmt.Sprintf("Failed to create policy at precedence %d: %s", precedence, cf.SanitizeErrorMessage(err)))
				continue
			}
			policyID = result.ID
		}

		resolvedStatuses = append(resolvedStatuses, networkingv1alpha2.ResolvedPolicyStatus{
			Precedence: precedence,
			PolicyID:   policyID,
			GroupID:    policy.groupID,
			GroupName:  policy.groupName,
			Source:     policy.source,
			Decision:   decision,
		})
	}

	// Delete orphaned policies (policies that exist in Cloudflare but not in desired)
	for precedence, orphan := range existingMap {
		if err := r.cfAPI.DeleteAccessPolicy(applicationID, orphan.ID); err != nil {
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete orphaned policy",
					"policyId", orphan.ID, "precedence", precedence)
			}
		} else {
			r.log.Info("Deleted orphaned policy", "policyId", orphan.ID, "precedence", precedence)
		}
	}

	return resolvedStatuses, nil
}

// cleanupAllPolicies removes all policies for an application.
//
//nolint:revive // cognitive complexity is acceptable for cleanup logic
func (r *Reconciler) cleanupAllPolicies(applicationID string) error {
	policies, err := r.cfAPI.ListAccessPolicies(applicationID)
	if err != nil {
		if cf.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to list policies for cleanup: %w", err)
	}

	for _, policy := range policies {
		if err := r.cfAPI.DeleteAccessPolicy(applicationID, policy.ID); err != nil {
			if !cf.IsNotFoundError(err) {
				r.log.Error(err, "failed to delete policy during cleanup", "policyId", policy.ID)
			}
		}
	}

	return nil
}

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

	// Find all AccessApplications that reference this IdP
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
			"accessapplication", app.Name,
			"namespace", app.Namespace)
		requests = append(requests, reconcile.Request{
			NamespacedName: apitypes.NamespacedName{
				Name:      app.Name,
				Namespace: app.Namespace,
			},
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

	// Find all AccessApplications that reference this AccessGroup
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
			NamespacedName: apitypes.NamespacedName{
				Name:      app.Name,
				Namespace: app.Namespace,
			},
		})
	}

	return requests
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessapplication-controller")
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
		Complete(r)
}
