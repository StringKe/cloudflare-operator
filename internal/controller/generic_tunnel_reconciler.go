package controller

import (
	"errors"
	"fmt"
	"time"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/clients/k8s"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CredentialsJsonFilename string = "credentials.json"
	CloudflaredLatestImage  string = "cloudflare/cloudflared:latest"
)

type GenericTunnelReconciler interface {
	k8s.GenericReconciler

	GetScheme() *runtime.Scheme
	GetTunnel() Tunnel
	GetCfAPI() *cf.API
	SetCfAPI(*cf.API)
	GetCfSecret() *corev1.Secret
	GetTunnelCreds() string
	SetTunnelCreds(string)
}

func TunnelNamespacedName(r GenericTunnelReconciler) apitypes.NamespacedName {
	return apitypes.NamespacedName{Name: r.GetTunnel().GetName(), Namespace: r.GetTunnel().GetNamespace()}
}

// getSecretFinalizerName returns a unique finalizer name for the Secret tied to this Tunnel.
// This supports multiple Tunnels sharing a Secret (PR #158 fix).
func getSecretFinalizerName(tunnelName string) string {
	return secretFinalizerPrefix + tunnelName
}

// labelsForTunnel returns the labels for selecting the resources
// belonging to the given Tunnel CR name.
func labelsForTunnel(cf Tunnel) map[string]string {
	return map[string]string{
		tunnelLabel:          cf.GetName(),
		tunnelAppLabel:       "cloudflared",
		tunnelIdLabel:        cf.GetStatus().TunnelId,
		tunnelNameLabel:      cf.GetStatus().TunnelName,
		tunnelDomainLabel:    cf.GetSpec().Cloudflare.Domain,
		isClusterTunnelLabel: "false",
	}
}

// handleTunnelConflict handles the case where tunnel creation failed due to conflict.
// Returns nil if tunnel was successfully adopted, error otherwise.
//
//nolint:revive // secretExists is necessary to determine adoption success
func handleTunnelConflict(r GenericTunnelReconciler, createErr error, secretExists bool) error {
	r.GetLog().Info("Tunnel already exists (concurrent creation detected), attempting to adopt")
	retryTunnelID, retryErr := r.GetCfAPI().GetTunnelId()
	if retryErr != nil || retryTunnelID == "" {
		r.GetLog().Error(createErr, "Tunnel creation conflict but unable to find tunnel")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"FailedCreate", "Tunnel creation conflict - unable to resolve")
		return createErr
	}

	r.GetLog().Info("Successfully found tunnel after conflict, adopting", "tunnelId", retryTunnelID)
	r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
		"Adopted", "Adopted tunnel after concurrent creation conflict")

	if !secretExists {
		err := fmt.Errorf("tunnel %s was created but credentials are missing", retryTunnelID)
		r.GetLog().Error(err, "Cannot recover tunnel without credentials")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"AdoptionFailed", "Tunnel exists but credentials are lost - manual intervention required")
		return err
	}
	return nil
}

// createOrAdoptTunnel attempts to create a new tunnel or adopt an existing one on conflict.
func createOrAdoptTunnel(r GenericTunnelReconciler, secretExists bool) error {
	_, creds, createErr := r.GetCfAPI().CreateTunnel()
	if createErr == nil {
		r.GetLog().Info("Tunnel created on Cloudflare")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
			"Created", "Tunnel created successfully on Cloudflare")
		r.SetTunnelCreds(creds)
		return nil
	}

	// P0 FIX: Handle "tunnel already exists" error gracefully
	// This can happen when concurrent reconciles race
	if cf.IsConflictError(createErr) {
		return handleTunnelConflict(r, createErr, secretExists)
	}

	r.GetLog().Error(createErr, "unable to create Tunnel")
	r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
		"FailedCreate", "Unable to create Tunnel on Cloudflare")
	return createErr
}

func setupTunnel(r GenericTunnelReconciler) (ctrl.Result, bool, error) {
	okNewTunnel := r.GetTunnel().GetSpec().NewTunnel != nil
	okExistingTunnel := r.GetTunnel().GetSpec().ExistingTunnel != nil

	// If both are set (or neither are), we have a problem
	if okNewTunnel == okExistingTunnel {
		err := fmt.Errorf("spec ExistingTunnel and NewTunnel cannot be both empty and are mutually exclusive")
		r.GetLog().Error(err, "spec ExistingTunnel and NewTunnel cannot be both empty and are mutually exclusive")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "ErrSpecTunnel", "ExistingTunnel and NewTunnel cannot be both empty and are mutually exclusive")
		return ctrl.Result{}, false, err
	}

	if okExistingTunnel {
		// Existing Tunnel, Set tunnelId in status and get creds file
		if err := setupExistingTunnel(r); err != nil {
			return ctrl.Result{}, false, err
		}
	} else {
		// New tunnel, finalizer/cleanup logic + creation
		if r.GetTunnel().GetObject().GetDeletionTimestamp() != nil {
			if res, ok, err := cleanupTunnel(r); !ok {
				return res, false, err
			}
		} else {
			if err := setupNewTunnel(r); err != nil {
				return ctrl.Result{}, false, err
			}
		}
	}

	return ctrl.Result{}, true, nil
}

func setupExistingTunnel(r GenericTunnelReconciler) error {
	cfAPI := r.GetCfAPI()
	cfAPI.TunnelName = r.GetTunnel().GetSpec().ExistingTunnel.Name
	cfAPI.TunnelId = r.GetTunnel().GetSpec().ExistingTunnel.Id
	r.SetCfAPI(cfAPI)

	// Read secret for credentials file
	cfCredFileB64, okCredFile := r.GetCfSecret().Data[r.GetTunnel().GetSpec().Cloudflare.CLOUDFLARE_TUNNEL_CREDENTIAL_FILE]
	cfSecretB64, okSecret := r.GetCfSecret().Data[r.GetTunnel().GetSpec().Cloudflare.CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET]

	if !okCredFile && !okSecret {
		err := fmt.Errorf("neither key not found in secret")
		r.GetLog().Error(err, "neither key not found in secret", "secret", r.GetTunnel().GetSpec().Cloudflare.Secret, "key1", r.GetTunnel().GetSpec().Cloudflare.CLOUDFLARE_TUNNEL_CREDENTIAL_FILE, "key2", r.GetTunnel().GetSpec().Cloudflare.CLOUDFLARE_TUNNEL_CREDENTIAL_SECRET)
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "ErrSpecSecret", "Neither Key found in Secret")
		return err
	}

	if okCredFile {
		r.SetTunnelCreds(string(cfCredFileB64))
	} else {
		creds, err := r.GetCfAPI().GetTunnelCreds(string(cfSecretB64))
		if err != nil {
			r.GetLog().Error(err, "error getting tunnel credentials from secret")
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "ErrSpecApi", "Error in getting Tunnel Credentials from Secret")
			return err
		}
		r.SetTunnelCreds(creds)
	}

	return nil
}

func setupNewTunnel(r GenericTunnelReconciler) error {
	// New tunnel, not yet setup, create on Cloudflare
	if r.GetTunnel().GetStatus().TunnelId == "" {
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "Creating", "Tunnel is being created")
		r.GetCfAPI().TunnelName = r.GetTunnel().GetSpec().NewTunnel.Name

		// Check if we already have a secret with credentials (from a previous partial reconcile)
		secret := &corev1.Secret{}
		secretExists := false
		if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), secret); err == nil {
			if creds, ok := secret.Data[CredentialsJsonFilename]; ok && len(creds) > 0 {
				secretExists = true
				r.SetTunnelCreds(string(creds))
				r.GetLog().Info("Found existing credentials secret, will try to adopt tunnel")
			}
		}

		// Try to find existing tunnel with this name first (adoption logic)
		existingTunnelID, err := r.GetCfAPI().GetTunnelId()
		if err == nil && existingTunnelID != "" {
			// Found existing tunnel, adopt it
			r.GetLog().Info("Found existing tunnel with same name, adopting",
				"tunnelId", existingTunnelID, "tunnelName", r.GetCfAPI().ValidTunnelName)
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
				"Adopted", "Adopted existing tunnel from Cloudflare")

			if !secretExists {
				// We found a tunnel but don't have credentials - this is a problem
				// The tunnel might have been created by this operator but the secret was deleted
				// We cannot recover without deleting and recreating the tunnel
				err := fmt.Errorf("found existing tunnel %s but no credentials secret", existingTunnelID)
				r.GetLog().Error(err, "Cannot adopt tunnel without credentials")
				r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
					"AdoptionFailed", "Found tunnel but missing credentials secret")
				return err
			}
		} else {
			// Create new tunnel
			if err := createOrAdoptTunnel(r, secretExists); err != nil {
				return err
			}
		}

		// CRITICAL: Update status immediately after tunnel creation/adoption
		// This prevents duplicate creation attempts on subsequent reconciles
		// P0 FIX: Retry status update with conflict handling
		if err := updateTunnelStatusMinimalWithRetry(r); err != nil {
			r.GetLog().Error(err, "Failed to update tunnel status after creation")
			// Don't return error - the tunnel was created, we should continue
			// The status will be updated on the next reconcile
		}
	} else {
		// Read existing secret into tunnelCreds
		secret := &corev1.Secret{}
		if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), secret); err != nil {
			r.GetLog().Error(err, "Error in getting existing secret, tunnel restart will crash, please recreate tunnel")
		}
		r.SetTunnelCreds(string(secret.Data[CredentialsJsonFilename]))
	}

	// Add finalizer for tunnel
	if !controllerutil.ContainsFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer) {
		controllerutil.AddFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer)
		if err := r.GetClient().Update(r.GetContext(), r.GetTunnel().GetObject()); err != nil {
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "FailedFinalizerSet", "Failed to add Tunnel Finalizer")
			return err
		}
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "FinalizerSet", "Tunnel Finalizer added")
	}
	return nil
}

// updateTunnelStatusMinimal updates only the essential tunnel status fields (TunnelId, TunnelName, AccountId)
// This is called immediately after tunnel creation to prevent duplicate creation on re-reconcile
func updateTunnelStatusMinimal(r GenericTunnelReconciler) error {
	status := r.GetTunnel().GetStatus()
	status.AccountId = r.GetCfAPI().ValidAccountId
	status.TunnelId = r.GetCfAPI().ValidTunnelId
	status.TunnelName = r.GetCfAPI().ValidTunnelName
	status.State = "creating"
	status.ObservedGeneration = r.GetTunnel().GetObject().GetGeneration()

	// Set condition for creating state
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Creating",
		Message:            "Tunnel is being created",
		ObservedGeneration: r.GetTunnel().GetObject().GetGeneration(),
	})

	r.GetTunnel().SetStatus(status)

	if err := r.GetClient().Status().Update(r.GetContext(), r.GetTunnel().GetObject()); err != nil {
		r.GetLog().Error(err, "Failed to update Tunnel status",
			"namespace", r.GetTunnel().GetNamespace(), "name", r.GetTunnel().GetName())
		return err
	}
	r.GetLog().Info("Tunnel status updated with tunnel ID", "tunnelId", status.TunnelId)
	return nil
}

// applyTunnelStatusCreating applies the "creating" status to the tunnel
func applyTunnelStatusCreating(r GenericTunnelReconciler) {
	status := r.GetTunnel().GetStatus()
	status.AccountId = r.GetCfAPI().ValidAccountId
	status.TunnelId = r.GetCfAPI().ValidTunnelId
	status.TunnelName = r.GetCfAPI().ValidTunnelName
	status.State = "creating"
	status.ObservedGeneration = r.GetTunnel().GetObject().GetGeneration()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Creating",
		Message:            "Tunnel is being created",
		ObservedGeneration: r.GetTunnel().GetObject().GetGeneration(),
	})

	r.GetTunnel().SetStatus(status)
}

// refetchTunnelForRetry re-fetches the tunnel object to get the latest ResourceVersion
func refetchTunnelForRetry(r GenericTunnelReconciler) error {
	if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), r.GetTunnel().GetObject()); err != nil {
		r.GetLog().Error(err, "Failed to re-fetch tunnel for status update retry")
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

// updateTunnelStatusMinimalWithRetry updates tunnel status with retry on conflict
// P0 FIX: This ensures status is updated even when concurrent reconciles race
//
//nolint:revive // retry loop complexity is inherent to the logic
func updateTunnelStatusMinimalWithRetry(r GenericTunnelReconciler) error {
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			if err := refetchTunnelForRetry(r); err != nil {
				return err
			}
		}

		applyTunnelStatusCreating(r)

		err := r.GetClient().Status().Update(r.GetContext(), r.GetTunnel().GetObject())
		if err == nil {
			r.GetLog().Info("Tunnel status updated with tunnel ID", "tunnelId", r.GetTunnel().GetStatus().TunnelId)
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
		r.GetLog().Info("Tunnel status update conflict, retrying", "attempt", i+1)
		lastErr = err
	}

	return fmt.Errorf("failed to update tunnel status after %d retries: %w", maxRetries, lastErr)
}

// applyTunnelState applies the state and condition to the tunnel object in memory
func applyTunnelState(r GenericTunnelReconciler, state string, conditionStatus metav1.ConditionStatus, reason, message string) {
	status := r.GetTunnel().GetStatus()
	status.State = state
	status.ObservedGeneration = r.GetTunnel().GetObject().GetGeneration()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: r.GetTunnel().GetObject().GetGeneration(),
	})

	r.GetTunnel().SetStatus(status)
}

// setTunnelState sets the tunnel state and condition with retry on conflict (best effort, logs errors but doesn't return them)
// P0 FIX: Added retry logic for status update to handle concurrent reconciles
//
//nolint:revive // retry loop complexity is inherent to the logic
func setTunnelState(r GenericTunnelReconciler, state string, conditionStatus metav1.ConditionStatus, reason, message string) {
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			if err := refetchTunnelForRetry(r); err != nil {
				r.GetLog().Error(err, "Failed to refetch tunnel for state update retry")
				return
			}
		}

		applyTunnelState(r, state, conditionStatus, reason, message)

		err := r.GetClient().Status().Update(r.GetContext(), r.GetTunnel().GetObject())
		if err == nil {
			return
		}
		if !apierrors.IsConflict(err) {
			r.GetLog().Error(err, "Failed to update tunnel state", "state", state)
			return
		}
		r.GetLog().Info("Tunnel state update conflict, retrying", "attempt", i+1, "state", state)
		lastErr = err
	}

	if lastErr != nil {
		r.GetLog().Error(lastErr, "Failed to update tunnel state after retries", "state", state)
	}
}

func cleanupTunnel(r GenericTunnelReconciler) (ctrl.Result, bool, error) {
	if controllerutil.ContainsFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer) {
		// Run finalization logic. If the finalization logic fails,
		// don't remove the finalizer so that we can retry during the next reconciliation.

		r.GetLog().Info("starting deletion cycle")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "Deleting", "Starting Tunnel Deletion")

		// Set deleting state (best effort, don't block on failure)
		setTunnelState(r, "deleting", metav1.ConditionFalse, "Deleting", "Tunnel is being deleted")
		cfDeployment := &appsv1.Deployment{}
		var bypass bool
		if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), cfDeployment); err != nil {
			r.GetLog().Error(err, "Error in getting deployments, might already be deleted?")
			bypass = true
		}
		if !bypass && *cfDeployment.Spec.Replicas != 0 {
			r.GetLog().Info("Scaling down cloudflared")
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "Scaling", "Scaling down cloudflared")
			var size int32 = 0
			cfDeployment.Spec.Replicas = &size
			if err := r.GetClient().Update(r.GetContext(), cfDeployment); err != nil {
				r.GetLog().Error(err, "Failed to update Deployment", "Deployment.Namespace", cfDeployment.Namespace, "Deployment.Name", cfDeployment.Name)
				r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "FailedScaling", "Failed to scale down cloudflared")
				return ctrl.Result{}, false, err
			}
			r.GetLog().Info("Scaling down successful")
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "Scaled", "Scaling down cloudflared successful")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
		}
		if bypass || *cfDeployment.Spec.Replicas == 0 {
			// P0 FIX: Delete all routes associated with this tunnel BEFORE deleting the tunnel
			// This prevents the "Cannot delete tunnel because it has Warp routing configured" error
			tunnelID := r.GetTunnel().GetStatus().TunnelId
			if tunnelID != "" {
				deletedCount, err := r.GetCfAPI().DeleteTunnelRoutesByTunnelID(tunnelID)
				if err != nil {
					r.GetLog().Error(err, "Failed to delete tunnel routes", "tunnelId", tunnelID)
					r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
						"FailedDeletingRoutes", fmt.Sprintf("Failed to delete tunnel routes: %v", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, false, err
				}
				if deletedCount > 0 {
					r.GetLog().Info("Deleted tunnel routes before tunnel deletion", "tunnelId", tunnelID, "count", deletedCount)
					r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
						"RoutesDeleted", fmt.Sprintf("Deleted %d tunnel routes", deletedCount))
				}
			}

			// P0 FIX: Improve deletion idempotency
			// Handle case where tunnel is already deleted from Cloudflare
			if err := r.GetCfAPI().DeleteTunnel(); err != nil {
				// P0 FIX: Check if tunnel is already deleted (NotFound error)
				if !cf.IsNotFoundError(err) {
					r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
						"FailedDeleting", fmt.Sprintf("Tunnel deletion failed: %v", cf.SanitizeErrorMessage(err)))
					return ctrl.Result{RequeueAfter: 30 * time.Second}, false, err
				}
				r.GetLog().Info("Tunnel already deleted from Cloudflare, continuing with cleanup",
					"tunnelID", r.GetTunnel().GetStatus().TunnelId)
				r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
					"AlreadyDeleted", "Tunnel was already deleted from Cloudflare")
			} else {
				r.GetLog().Info("Tunnel deleted", "tunnelID", r.GetTunnel().GetStatus().TunnelId)
				r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "Deleted", "Tunnel deletion successful")
			}

			// PR #158 fix: Remove Secret finalizer BEFORE tunnel finalizer
			// This ensures the Secret can be cleaned up properly
			if err := removeSecretFinalizer(r); err != nil {
				// Log but don't block - Secret might have been force-deleted
				r.GetLog().Error(err, "Failed to remove Secret finalizer, continuing with tunnel cleanup")
			}

			// Remove tunnelFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			// P0 FIX: Use retry for finalizer removal
			controllerutil.RemoveFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer)
			err := r.GetClient().Update(r.GetContext(), r.GetTunnel().GetObject())
			if err != nil {
				if apierrors.IsConflict(err) {
					// Conflict - requeue to retry
					r.GetLog().Info("Finalizer removal conflict, will retry")
					return ctrl.Result{RequeueAfter: time.Second}, false, nil
				}
				r.GetLog().Error(err, "unable to continue with tunnel deletion")
				r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "FailedFinalizerUnset", "Unable to remove Tunnel Finalizer")
				return ctrl.Result{}, false, err
			}
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "FinalizerUnset", "Tunnel Finalizer removed")
			return ctrl.Result{}, true, nil
		}
	}
	return ctrl.Result{}, true, nil
}

// removeSecretFinalizer removes the tunnel-specific finalizer from the managed Secret.
// Handles NotFound gracefully (Secret might have been force-deleted).
// This implements PR #158 fix.
func removeSecretFinalizer(r GenericTunnelReconciler) error {
	// Only NewTunnel creates managed Secrets with finalizers
	if r.GetTunnel().GetSpec().NewTunnel == nil {
		return nil
	}

	secret := &corev1.Secret{}
	if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret already deleted, nothing to do
			r.GetLog().Info("Secret already deleted, skipping finalizer removal")
			return nil
		}
		return err
	}

	finalizerName := getSecretFinalizerName(r.GetTunnel().GetName())
	if controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.RemoveFinalizer(secret, finalizerName)
		if err := r.GetClient().Update(r.GetContext(), secret); err != nil {
			if apierrors.IsNotFound(err) {
				// Secret deleted between Get and Update, that's fine
				return nil
			}
			return err
		}
		r.GetLog().Info("Removed finalizer from Secret", "finalizer", finalizerName)
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "SecretFinalizerRemoved", "Removed finalizer from managed Secret")
	}
	return nil
}

// ensureSecretFinalizer adds a tunnel-specific finalizer to the managed Secret.
// This prevents the Secret from being deleted while the Tunnel still needs it.
// This implements PR #158 fix.
func ensureSecretFinalizer(r GenericTunnelReconciler) error {
	secret := &corev1.Secret{}
	if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), secret); err != nil {
		return err
	}

	finalizerName := getSecretFinalizerName(r.GetTunnel().GetName())
	if !controllerutil.ContainsFinalizer(secret, finalizerName) {
		controllerutil.AddFinalizer(secret, finalizerName)
		if err := r.GetClient().Update(r.GetContext(), secret); err != nil {
			r.GetLog().Error(err, "Failed to add finalizer to Secret")
			return err
		}
		r.GetLog().Info("Added finalizer to Secret", "finalizer", finalizerName)
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal, "SecretFinalizerSet", "Added finalizer to managed Secret")
	}
	return nil
}

// applyTunnelStatusActive applies the "active" status fields to the tunnel object in memory
func applyTunnelStatusActive(r GenericTunnelReconciler) {
	status := r.GetTunnel().GetStatus()
	status.AccountId = r.GetCfAPI().ValidAccountId
	status.TunnelId = r.GetCfAPI().ValidTunnelId
	status.TunnelName = r.GetCfAPI().ValidTunnelName
	status.ZoneId = r.GetCfAPI().ValidZoneId
	status.State = "active"
	status.ObservedGeneration = r.GetTunnel().GetObject().GetGeneration()

	// Set condition for ready state
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Tunnel is active and ready",
		ObservedGeneration: r.GetTunnel().GetObject().GetGeneration(),
	})

	r.GetTunnel().SetStatus(status)
}

// updateTunnelStatus updates the tunnel status with retry on conflict
// P0 FIX: Added retry logic for status update to handle concurrent reconciles
//
//nolint:revive // function length is acceptable for reconciliation logic
func updateTunnelStatus(r GenericTunnelReconciler) error {
	labels := r.GetTunnel().GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	for k, v := range labelsForTunnel(r.GetTunnel()) {
		labels[k] = v
	}
	r.GetTunnel().SetLabels(labels)
	if err := r.GetClient().Update(r.GetContext(), r.GetTunnel().GetObject()); err != nil {
		return err
	}

	// Validate Account and Tunnel (required)
	if _, err := r.GetCfAPI().GetAccountId(); err != nil {
		r.GetLog().Error(err, "Failed to validate Account ID")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"ErrSpecApi", "Error validating Cloudflare Account ID")
		return err
	}
	if _, err := r.GetCfAPI().GetTunnelId(); err != nil {
		r.GetLog().Error(err, "Failed to validate Tunnel ID")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"ErrSpecApi", "Error validating Cloudflare Tunnel ID")
		return err
	}

	// Validate Zone (optional - only if domain is specified)
	// Zone is only needed for DNS record management, not for tunnel operation
	if r.GetCfAPI().Domain != "" {
		if _, err := r.GetCfAPI().GetZoneId(); err != nil {
			r.GetLog().Info("Zone validation failed, DNS features may not work",
				"domain", r.GetCfAPI().Domain, "error", err.Error())
			// Don't return error - tunnel can still work without zone
		}
	}

	// P0 FIX: Sync warp-routing configuration to Cloudflare API
	// This ensures that when enableWarpRouting is set on Tunnel/ClusterTunnel,
	// the configuration is actually synced to Cloudflare's remote config.
	// Without this, cloudflared in --token mode would never receive the warp-routing setting.
	if err := syncWarpRoutingConfig(r); err != nil {
		r.GetLog().Error(err, "Failed to sync warp-routing configuration")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"WarpRoutingSyncFailed", fmt.Sprintf("Failed to sync warp-routing config: %v", cf.SanitizeErrorMessage(err)))
		// Don't return error - tunnel can still work, warp-routing will be synced on next reconcile
	}

	// P0 FIX: Update status with retry on conflict
	const maxRetries = 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			if err := refetchTunnelForRetry(r); err != nil {
				return err
			}
		}

		applyTunnelStatusActive(r)

		err := r.GetClient().Status().Update(r.GetContext(), r.GetTunnel().GetObject())
		if err == nil {
			r.GetLog().Info("Tunnel status is set", "status", r.GetTunnel().GetStatus())
			return nil
		}
		if !apierrors.IsConflict(err) {
			r.GetLog().Error(err, "Failed to update Tunnel status",
				"namespace", r.GetTunnel().GetNamespace(), "name", r.GetTunnel().GetName())
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
				"FailedStatusSet", "Failed to set Tunnel status required for operation")
			return err
		}
		r.GetLog().Info("Tunnel status update conflict, retrying", "attempt", i+1)
		lastErr = err
	}

	r.GetLog().Error(lastErr, "Failed to update Tunnel status after retries",
		"namespace", r.GetTunnel().GetNamespace(), "name", r.GetTunnel().GetName())
	r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
		"FailedStatusSet", "Failed to set Tunnel status after retries")
	return fmt.Errorf("failed to update tunnel status after %d retries: %w", maxRetries, lastErr)
}

// syncWarpRoutingConfig syncs the warp-routing configuration from Tunnel/ClusterTunnel spec
// to Cloudflare's remote configuration using read-merge-write pattern.
//
// This is critical for --token mode where cloudflared pulls configuration from Cloudflare cloud
// instead of local ConfigMap. Uses MergeAndSync to avoid race conditions with other controllers
// (Ingress, Gateway, TunnelBinding) that might also be updating the tunnel configuration.
//
// The Tunnel/ClusterTunnel controller is the ONLY source of truth for:
// - warp-routing configuration
// - fallback target
//
// Other controllers (Ingress, Gateway, TunnelBinding) should preserve existing warp-routing
// by passing WarpRouting: nil to MergeAndSync.
func syncWarpRoutingConfig(r GenericTunnelReconciler) error {
	tunnelID := r.GetTunnel().GetStatus().TunnelId
	if tunnelID == "" {
		return nil // Tunnel not yet created, skip sync
	}

	enableWarpRouting := r.GetTunnel().GetSpec().EnableWarpRouting
	fallbackTarget := r.GetTunnel().GetSpec().FallbackTarget
	if fallbackTarget == "" {
		fallbackTarget = "http_status:404"
	}

	// Build source identifier for logging and debugging
	tunnelKind := "Tunnel"
	if r.GetTunnel().GetNamespace() == "" {
		tunnelKind = "ClusterTunnel"
	}
	source := fmt.Sprintf("%s/%s", tunnelKind, r.GetTunnel().GetName())

	r.GetLog().Info("Syncing warp-routing and fallback configuration using MergeAndSync",
		"tunnelId", tunnelID, "source", source, "enableWarpRouting", enableWarpRouting, "fallbackTarget", fallbackTarget)

	// Use MergeAndSync to safely update warp-routing and fallback without overwriting ingress rules
	// The Tunnel controller doesn't add any ingress rules - it only controls warp-routing and fallback
	// PreviousHostnames is empty because Tunnel controller doesn't own any hostnames
	mergeOpts := cf.MergeOptions{
		Source:            source,
		PreviousHostnames: r.GetTunnel().GetStatus().SyncedHostnames, // Should be empty for Tunnel controller
		CurrentRules:      nil,                                       // Tunnel controller doesn't add ingress rules
		WarpRouting:       &cf.WarpRoutingConfig{Enabled: enableWarpRouting},
		FallbackTarget:    fallbackTarget,
	}

	result, err := r.GetCfAPI().MergeAndSync(tunnelID, mergeOpts)
	if err != nil {
		return fmt.Errorf("failed to merge and sync tunnel configuration: %w", err)
	}

	r.GetLog().Info("Successfully synced warp-routing and fallback configuration",
		"tunnelId", tunnelID, "source", source, "configVersion", result.Version,
		"enableWarpRouting", enableWarpRouting, "fallbackTarget", fallbackTarget)
	r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
		"ConfigSynced", fmt.Sprintf("Tunnel configuration synced: warp-routing=%v, fallback=%s", enableWarpRouting, fallbackTarget))

	// Update tunnel status with config version (SyncedHostnames should remain empty for Tunnel controller)
	status := r.GetTunnel().GetStatus()
	status.ConfigVersion = result.Version
	status.SyncedHostnames = result.SyncedHostnames // Should be empty
	r.GetTunnel().SetStatus(status)
	// Note: Status will be saved in updateTunnelStatus which calls this function

	return nil
}

func createManagedResources(r GenericTunnelReconciler) (ctrl.Result, error) {
	// Check if Secret already exists, else create it
	// Skip breaking secret if tunnel creds is empty, something went wrong
	if r.GetTunnelCreds() != "" {
		if err := k8s.Apply(r, secretForTunnel(r)); err != nil {
			return ctrl.Result{}, err
		}
		// PR #158 fix: Add finalizer to Secret to prevent deletion while Tunnel needs it
		// Only for NewTunnel (managed Secrets), not ExistingTunnel (user-provided Secrets)
		if r.GetTunnel().GetSpec().NewTunnel != nil {
			if err := ensureSecretFinalizer(r); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		r.GetLog().Error(errors.New("empty tunnel creds"), "skipping updating the tunnel secret")
	}

	// Get tunnel token for remotely-managed mode
	// Token is used to start cloudflared with --token flag
	tunnelID := r.GetTunnel().GetStatus().TunnelId
	token, err := r.GetCfAPI().GetTunnelToken(tunnelID)
	if err != nil {
		r.GetLog().Error(err, "failed to get tunnel token", "tunnelId", tunnelID)
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "FailedGetToken", "Failed to get tunnel token from Cloudflare")
		return ctrl.Result{}, err
	}

	// Create token Secret for cloudflared to use
	tokenSecret := tokenSecretForTunnel(r, token)
	if err := k8s.Apply(r, tokenSecret); err != nil {
		return ctrl.Result{}, err
	}

	// Apply patch to deployment
	dep := deploymentForTunnel(r)
	if err := k8s.StrategicPatch(dep, r.GetTunnel().GetSpec().DeployPatch, dep); err != nil {
		r.GetLog().Error(err, "unable to patch deployment, check patch")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "FailedPatch", "Failed to patch deployment, check patch")
		return ctrl.Result{}, err
	}

	if err := k8s.Apply(r, dep); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// tokenSecretForTunnel returns a Secret containing the tunnel token for cloudflared --token mode
func tokenSecretForTunnel(r GenericTunnelReconciler, token string) *corev1.Secret {
	ls := labelsForTunnel(r.GetTunnel())
	sec := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.GetTunnel().GetName() + "-token",
			Namespace: r.GetTunnel().GetNamespace(),
			Labels:    ls,
		},
		StringData: map[string]string{"token": token},
	}
	// Set Tunnel instance as the owner and controller
	_ = ctrl.SetControllerReference(r.GetTunnel().GetObject(), sec, r.GetScheme())
	return sec
}

// secretForTunnel returns a tunnel Secret object
func secretForTunnel(r GenericTunnelReconciler) *corev1.Secret {
	ls := labelsForTunnel(r.GetTunnel())
	sec := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.GetTunnel().GetName(),
			Namespace: r.GetTunnel().GetNamespace(),
			Labels:    ls,
		},
		StringData: map[string]string{CredentialsJsonFilename: r.GetTunnelCreds()},
	}
	// Set Tunnel instance as the owner and controller
	ctrl.SetControllerReference(r.GetTunnel().GetObject(), sec, r.GetScheme())
	return sec
}

// deploymentForTunnel returns a tunnel Deployment object using token mode.
// In token mode, cloudflared uses --token flag and pulls configuration from Cloudflare cloud automatically.
// This eliminates the need for local ConfigMap and enables real-time configuration updates.
func deploymentForTunnel(r GenericTunnelReconciler) *appsv1.Deployment {
	ls := labelsForTunnel(r.GetTunnel())
	protocol := r.GetTunnel().GetSpec().Protocol

	// Token mode: cloudflared uses --token to authenticate and pulls config from cloud
	args := []string{
		"tunnel",
		"--protocol", protocol,
		"--metrics", "0.0.0.0:2000",
		"run",
		"--token", "$(TUNNEL_TOKEN)",
	}

	// Environment variables - TUNNEL_TOKEN from Secret
	envVars := []corev1.EnvVar{{
		Name: "TUNNEL_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.GetTunnel().GetName() + "-token",
				},
				Key: "token",
			},
		},
	}}

	// Volumes - only certs if needed (no ConfigMap needed in token mode)
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	if r.GetTunnel().GetSpec().OriginCaPool != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "certs",
			MountPath: "/etc/cloudflared/certs",
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  r.GetTunnel().GetSpec().OriginCaPool,
					DefaultMode: ptr.To(int32(420)),
				},
			},
		})
	}

	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.GetTunnel().GetName(),
			Namespace: r.GetTunnel().GetNamespace(),
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image: CloudflaredLatestImage,
						Name:  "cloudflared",
						Args:  args,
						Env:   envVars,
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/ready",
									Port: intstr.IntOrString{IntVal: 2000},
								},
							},
							FailureThreshold:    1,
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "metrics",
								ContainerPort: 2000,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						VolumeMounts: volumeMounts,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							RunAsUser:                ptr.To(int64(1002)),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
					}},
					Volumes: volumes,
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/arch",
												Operator: corev1.NodeSelectorOpIn,
												Values: []string{
													"amd64",
													"arm64",
												},
											},
											{
												Key:      "kubernetes.io/os",
												Operator: corev1.NodeSelectorOpIn,
												Values: []string{
													"linux",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	// Set Tunnel instance as the owner and controller
	ctrl.SetControllerReference(r.GetTunnel().GetObject(), dep, r.GetScheme())
	return dep
}
