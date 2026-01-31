package controller

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
	"github.com/StringKe/cloudflare-operator/internal/clients/k8s"
	"github.com/StringKe/cloudflare-operator/internal/controller/tunnelconfig"
	"github.com/StringKe/cloudflare-operator/internal/service"
	tunnelsvc "github.com/StringKe/cloudflare-operator/internal/service/tunnel"

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

	// Tunnel kind constants for SyncState source identification
	kindTunnel        = "Tunnel"
	kindClusterTunnel = "ClusterTunnel"
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
	ctx := r.GetContext()

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
		creds, err := r.GetCfAPI().GetTunnelCreds(ctx, string(cfSecretB64))
		if err != nil {
			r.GetLog().Error(err, "error getting tunnel credentials from secret")
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "ErrSpecApi", "Error in getting Tunnel Credentials from Secret")
			return err
		}
		r.SetTunnelCreds(creds)
	}

	return nil
}

// setupNewTunnel sets up a new tunnel using the SyncState-based lifecycle pattern.
// This follows the six-layer architecture where:
// - L2 Controller requests operations via L3 Service
// - L5 Sync Controller performs actual API calls
// - Results are stored in SyncState.Status.ResultData
//
//nolint:revive // cognitive complexity is acceptable for state machine logic
func setupNewTunnel(r GenericTunnelReconciler) error {
	tunnel := r.GetTunnel()
	ctx := r.GetContext()
	log := r.GetLog()

	// If tunnel already has an ID, it's already created - just load credentials
	if tunnel.GetStatus().TunnelId != "" {
		return loadExistingTunnelCredentials(r)
	}

	// Check current lifecycle state
	tunnelName := tunnel.GetSpec().NewTunnel.Name
	lifecycleSvc := tunnelsvc.NewLifecycleService(r.GetClient())

	// Check if we have a pending lifecycle operation
	result, err := lifecycleSvc.GetLifecycleResult(ctx, tunnelName)
	if err != nil {
		log.Error(err, "Failed to check lifecycle result")
		return err
	}

	// If we have a result, the tunnel was created - apply it
	if result != nil {
		log.Info("Tunnel lifecycle completed, applying result",
			"tunnelId", result.TunnelID,
			"tunnelName", result.TunnelName)
		return applyLifecycleResult(r, result)
	}

	// Check if there's an error from a previous attempt
	errMsg, err := lifecycleSvc.GetLifecycleError(ctx, tunnelName)
	if err != nil {
		log.Error(err, "Failed to check lifecycle error")
		return err
	}
	if errMsg != "" {
		log.Error(errors.New(errMsg), "Tunnel lifecycle failed")
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning,
			"LifecycleFailed", fmt.Sprintf("Tunnel creation failed: %s", errMsg))
		return fmt.Errorf("tunnel lifecycle failed: %s", errMsg)
	}

	// Check if lifecycle operation is already in progress
	completed, err := lifecycleSvc.IsLifecycleCompleted(ctx, tunnelName)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// If not completed but SyncState exists, operation is in progress - wait
	syncStateName := tunnelsvc.GetSyncStateName(tunnelName)
	syncState, _ := lifecycleSvc.GetSyncState(ctx, v1alpha2.SyncResourceTunnelLifecycle, syncStateName)
	if syncState != nil && !completed {
		log.Info("Tunnel lifecycle operation in progress, waiting",
			"syncState", syncStateName,
			"status", syncState.Status.SyncStatus)
		// Return special error to trigger requeue
		return &lifecyclePendingError{tunnelName: tunnelName}
	}

	// No lifecycle operation in progress, request tunnel creation
	log.Info("Requesting tunnel creation via SyncState", "tunnelName", tunnelName)
	r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal,
		"Creating", "Tunnel creation requested via SyncState")

	// Determine source kind
	tunnelKind := kindTunnel
	if tunnel.GetNamespace() == "" {
		tunnelKind = kindClusterTunnel
	}

	// Build credentials reference
	credRef := getCredentialsReference(r)

	opts := tunnelsvc.CreateTunnelOptions{
		TunnelName:     tunnelName,
		AccountID:      "", // Will be resolved by Sync Controller from credentials
		ConfigSrc:      "cloudflare",
		Source:         service.Source{Kind: tunnelKind, Namespace: tunnel.GetNamespace(), Name: tunnel.GetName()},
		CredentialsRef: credRef,
	}

	if _, err := lifecycleSvc.RequestCreate(ctx, opts); err != nil {
		log.Error(err, "Failed to request tunnel creation")
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning,
			"CreateFailed", fmt.Sprintf("Failed to request tunnel creation: %v", err))
		return err
	}

	// Update status to creating
	setTunnelState(r, "creating", metav1.ConditionFalse, "Creating", "Tunnel creation in progress")

	// Return special error to trigger requeue
	return &lifecyclePendingError{tunnelName: tunnelName}
}

// loadExistingTunnelCredentials loads credentials from the existing secret
func loadExistingTunnelCredentials(r GenericTunnelReconciler) error {
	secret := &corev1.Secret{}
	if err := r.GetClient().Get(r.GetContext(), TunnelNamespacedName(r), secret); err != nil {
		r.GetLog().Error(err, "Error getting existing secret, tunnel restart will crash")
		return nil // Don't fail the reconcile, just log
	}
	if creds, ok := secret.Data[CredentialsJsonFilename]; ok {
		r.SetTunnelCreds(string(creds))
	}

	// Ensure finalizer is set
	if !controllerutil.ContainsFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer) {
		controllerutil.AddFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer)
		if err := r.GetClient().Update(r.GetContext(), r.GetTunnel().GetObject()); err != nil {
			r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
				"FailedFinalizerSet", "Failed to add Tunnel Finalizer")
			return err
		}
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
			"FinalizerSet", "Tunnel Finalizer added")
	}

	return nil
}

// applyLifecycleResult applies the result from a completed lifecycle operation
//
// CRITICAL: Tunnel credentials and token are only returned ONCE during tunnel creation.
// They MUST be persisted immediately to Secrets before any operation that might fail.
// If we lose these credentials, the only recovery is to delete and recreate the tunnel.
//
//nolint:revive // cognitive complexity is acceptable for state transition logic
func applyLifecycleResult(r GenericTunnelReconciler, result *tunnelsvc.LifecycleResult) error {
	tunnel := r.GetTunnel()
	log := r.GetLog()
	ctx := r.GetContext()

	log.Info("Applying tunnel lifecycle result",
		"tunnelId", result.TunnelID,
		"tunnelName", result.TunnelName,
		"hasToken", result.TunnelToken != "",
		"hasCreds", result.Credentials != "")

	// PRIORITY 1: Immediately persist credentials to Secret
	// This MUST happen first - credentials are only returned once during tunnel creation
	if result.Credentials != "" {
		creds, err := base64.StdEncoding.DecodeString(result.Credentials)
		if err != nil {
			log.Error(err, "Failed to decode tunnel credentials")
			return fmt.Errorf("failed to decode tunnel credentials: %w", err)
		}
		r.SetTunnelCreds(string(creds))

		// Create credentials Secret immediately
		credSecret := secretForTunnelWithCreds(r, string(creds))
		if err := k8s.Apply(r, credSecret); err != nil {
			log.Error(err, "Failed to persist tunnel credentials Secret")
			return fmt.Errorf("failed to persist tunnel credentials: %w", err)
		}
		log.Info("Tunnel credentials persisted to Secret", "secret", credSecret.Name)
	}

	// PRIORITY 2: Immediately persist token to Secret
	// Token is also only returned once - persist it before any other operations
	if result.TunnelToken != "" {
		tokenSecret := tokenSecretForTunnelWithToken(r, result.TunnelToken)
		if err := k8s.Apply(r, tokenSecret); err != nil {
			log.Error(err, "Failed to persist tunnel token Secret")
			return fmt.Errorf("failed to persist tunnel token: %w", err)
		}
		log.Info("Tunnel token persisted to Secret", "secret", tokenSecret.Name)

		// Also store in annotation for quick access
		tunnel.SetAnnotations(mergeAnnotations(tunnel.GetAnnotations(), map[string]string{
			"cloudflare-operator.io/tunnel-token": result.TunnelToken,
		}))
	}

	// Update cfAPI with tunnel details for status update
	cfAPI := r.GetCfAPI()
	cfAPI.ValidTunnelId = result.TunnelID
	cfAPI.ValidTunnelName = result.TunnelName
	if result.AccountTag != "" {
		cfAPI.ValidAccountId = result.AccountTag
	}
	r.SetCfAPI(cfAPI)

	// Update status with tunnel ID
	if err := updateTunnelStatusMinimalWithRetry(r); err != nil {
		log.Error(err, "Failed to update tunnel status after lifecycle completion")
		return err
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(tunnel.GetObject(), tunnelFinalizer) {
		controllerutil.AddFinalizer(tunnel.GetObject(), tunnelFinalizer)
		if err := r.GetClient().Update(ctx, tunnel.GetObject()); err != nil {
			r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal,
				"FailedFinalizerSet", "Failed to add Tunnel Finalizer")
			return err
		}
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal,
			"FinalizerSet", "Tunnel Finalizer added")
	}

	return nil
}

// secretForTunnelWithCreds creates a credentials Secret with the provided credentials.
// This is used during lifecycle result application to immediately persist credentials.
func secretForTunnelWithCreds(r GenericTunnelReconciler, creds string) *corev1.Secret {
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
		StringData: map[string]string{CredentialsJsonFilename: creds},
	}
	_ = ctrl.SetControllerReference(r.GetTunnel().GetObject(), sec, r.GetScheme())
	return sec
}

// tokenSecretForTunnelWithToken creates a token Secret with the provided token.
// This is used during lifecycle result application to immediately persist token.
func tokenSecretForTunnelWithToken(r GenericTunnelReconciler, token string) *corev1.Secret {
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
	_ = ctrl.SetControllerReference(r.GetTunnel().GetObject(), sec, r.GetScheme())
	return sec
}

// lifecyclePendingError indicates a lifecycle operation is pending
type lifecyclePendingError struct {
	tunnelName string
}

func (e *lifecyclePendingError) Error() string {
	return fmt.Sprintf("tunnel lifecycle pending for %s", e.tunnelName)
}

// IsLifecyclePendingError checks if the error is a lifecycle pending error
func IsLifecyclePendingError(err error) bool {
	_, ok := err.(*lifecyclePendingError)
	return ok
}

// mergeAnnotations merges two annotation maps
func mergeAnnotations(existing, additional map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string)
	}
	for k, v := range additional {
		existing[k] = v
	}
	return existing
}

// updateTunnelStatusMinimal updates only the essential tunnel status fields (TunnelId, TunnelName, AccountId)
// This is called immediately after tunnel creation to prevent duplicate creation on re-reconcile
//
//nolint:unused // internal helper, may be used in future
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

// cleanupTunnel handles tunnel deletion using the SyncState-based lifecycle pattern.
// This follows the six-layer architecture where deletion is performed by L5 Sync Controller.
//
//nolint:revive // cognitive complexity is acceptable for deletion state machine
func cleanupTunnel(r GenericTunnelReconciler) (ctrl.Result, bool, error) {
	if !controllerutil.ContainsFinalizer(r.GetTunnel().GetObject(), tunnelFinalizer) {
		return ctrl.Result{}, true, nil
	}

	tunnel := r.GetTunnel()
	ctx := r.GetContext()
	log := r.GetLog()

	log.Info("Starting deletion cycle")
	r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal, "Deleting", "Starting Tunnel Deletion")

	// Set deleting state
	setTunnelState(r, "deleting", metav1.ConditionFalse, "Deleting", "Tunnel is being deleted")

	// Step 1: Scale down deployment
	cfDeployment := &appsv1.Deployment{}
	deploymentExists := true
	if err := r.GetClient().Get(ctx, TunnelNamespacedName(r), cfDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			deploymentExists = false
		} else {
			log.Error(err, "Error getting deployment")
		}
	}

	if deploymentExists && cfDeployment.Spec.Replicas != nil && *cfDeployment.Spec.Replicas != 0 {
		log.Info("Scaling down cloudflared")
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal, "Scaling", "Scaling down cloudflared")
		var size int32 = 0
		cfDeployment.Spec.Replicas = &size
		if err := r.GetClient().Update(ctx, cfDeployment); err != nil {
			log.Error(err, "Failed to scale down deployment")
			r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning, "FailedScaling", "Failed to scale down cloudflared")
			return ctrl.Result{}, false, err
		}
		log.Info("Scaling down successful")
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal, "Scaled", "Scaling down cloudflared successful")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
	}

	// Step 2: Deployment is scaled down or deleted, proceed with tunnel deletion
	tunnelID := tunnel.GetStatus().TunnelId
	tunnelName := getTunnelNameForDeletion(tunnel)

	// Remove from ConfigMap
	if tunnelID != "" {
		tunnelKind := kindTunnel
		if tunnel.GetNamespace() == "" {
			tunnelKind = kindClusterTunnel
		}

		// Get operator namespace for ConfigMap
		operatorNamespace := tunnel.GetNamespace()
		if operatorNamespace == "" {
			operatorNamespace = "cloudflare-operator-system"
		}

		writer := tunnelconfig.NewWriter(r.GetClient(), operatorNamespace)
		sourceKey := fmt.Sprintf("%s/%s/%s", tunnelKind, tunnel.GetNamespace(), tunnel.GetName())
		if err := writer.RemoveSourceConfig(ctx, tunnelID, sourceKey); err != nil {
			log.Error(err, "Failed to remove from ConfigMap", "tunnelId", tunnelID)
		} else {
			log.Info("Removed from ConfigMap", "tunnelId", tunnelID)
		}
	}

	// Step 3: Request tunnel deletion via LifecycleService (only for NewTunnel)
	if tunnel.GetSpec().NewTunnel != nil && tunnelID != "" {
		lifecycleSvc := tunnelsvc.NewLifecycleService(r.GetClient())

		// Check if deletion is already completed
		completed, err := lifecycleSvc.IsLifecycleCompleted(ctx, tunnelName)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to check lifecycle completion")
		}

		if !completed {
			// Check if deletion request already submitted
			syncStateName := tunnelsvc.GetSyncStateName(tunnelName)
			syncState, _ := lifecycleSvc.GetSyncState(ctx, v1alpha2.SyncResourceTunnelLifecycle, syncStateName)

			if syncState == nil {
				// Request deletion via LifecycleService
				log.Info("Requesting tunnel deletion via SyncState",
					"tunnelId", tunnelID, "tunnelName", tunnelName)

				tunnelKind := kindTunnel
				if tunnel.GetNamespace() == "" {
					tunnelKind = kindClusterTunnel
				}

				credRef := getCredentialsReference(r)
				opts := tunnelsvc.DeleteTunnelOptions{
					TunnelID:       tunnelID,
					TunnelName:     tunnelName,
					AccountID:      tunnel.GetStatus().AccountId,
					Source:         service.Source{Kind: tunnelKind, Namespace: tunnel.GetNamespace(), Name: tunnel.GetName()},
					CredentialsRef: credRef,
					CleanupRoutes:  true,
				}

				if _, err := lifecycleSvc.RequestDelete(ctx, opts); err != nil {
					log.Error(err, "Failed to request tunnel deletion, continuing with finalizer removal")
					r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning,
						"DeleteFailed", fmt.Sprintf("Failed to request tunnel deletion (will remove finalizer anyway): %v", err))
					// Don't block finalizer removal - tunnel may need manual cleanup in Cloudflare
					// Skip waiting for SyncState and proceed directly to finalizer removal
				} else {
					r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal,
						"DeletionRequested", "Tunnel deletion requested via SyncState")
					return ctrl.Result{RequeueAfter: tunnelLifecycleCheckInterval}, false, nil
				}
			} else {
				// Deletion in progress, check status
				if syncState.Status.SyncStatus == v1alpha2.SyncStatusError {
					errMsg := syncState.Status.Error
					log.Error(errors.New(errMsg), "Tunnel deletion failed, continuing with finalizer removal")
					r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning,
						"DeleteFailed", fmt.Sprintf("Tunnel deletion failed (will remove finalizer anyway): %s", errMsg))
					// Continue to finalizer removal
				} else if syncState.Status.SyncStatus != v1alpha2.SyncStatusSynced {
					// Still in progress
					log.Info("Tunnel deletion in progress, waiting",
						"status", syncState.Status.SyncStatus)
					return ctrl.Result{RequeueAfter: tunnelLifecycleCheckInterval}, false, nil
				}
			}

			// Cleanup lifecycle SyncState (best effort)
			if err := lifecycleSvc.CleanupSyncState(ctx, tunnelName); err != nil {
				log.Error(err, "Failed to cleanup lifecycle SyncState")
			}

			log.Info("Tunnel cleanup completed", "tunnelId", tunnelID)
			r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal, "Deleted", "Tunnel cleanup completed")
		}
	}

	// Step 4: Remove Secret finalizer
	if err := removeSecretFinalizer(r); err != nil {
		log.Error(err, "Failed to remove Secret finalizer, continuing with cleanup")
	}

	// Step 5: Remove tunnel finalizer
	controllerutil.RemoveFinalizer(tunnel.GetObject(), tunnelFinalizer)
	if err := r.GetClient().Update(ctx, tunnel.GetObject()); err != nil {
		if apierrors.IsConflict(err) {
			log.Info("Finalizer removal conflict, will retry")
			return ctrl.Result{RequeueAfter: time.Second}, false, nil
		}
		log.Error(err, "Unable to remove tunnel finalizer")
		r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeWarning, "FailedFinalizerUnset", "Unable to remove Tunnel Finalizer")
		return ctrl.Result{}, false, err
	}

	r.GetRecorder().Event(tunnel.GetObject(), corev1.EventTypeNormal, "FinalizerUnset", "Tunnel Finalizer removed")
	return ctrl.Result{}, true, nil
}

// getTunnelNameForDeletion returns the tunnel name for deletion operations
func getTunnelNameForDeletion(tunnel Tunnel) string {
	if tunnel.GetSpec().NewTunnel != nil {
		return tunnel.GetSpec().NewTunnel.Name
	}
	if tunnel.GetSpec().ExistingTunnel != nil {
		return tunnel.GetSpec().ExistingTunnel.Name
	}
	return tunnel.GetName()
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

	ctx := r.GetContext()

	// Validate Account and Tunnel (required)
	if _, err := r.GetCfAPI().GetAccountId(ctx); err != nil {
		r.GetLog().Error(err, "Failed to validate Account ID")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"ErrSpecApi", "Error validating Cloudflare Account ID")
		return err
	}
	if _, err := r.GetCfAPI().GetTunnelId(ctx); err != nil {
		r.GetLog().Error(err, "Failed to validate Tunnel ID")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning,
			"ErrSpecApi", "Error validating Cloudflare Tunnel ID")
		return err
	}

	// Validate Zone (optional - only if domain is specified)
	// Zone is only needed for DNS record management, not for tunnel operation
	if r.GetCfAPI().Domain != "" {
		if _, err := r.GetCfAPI().GetZoneId(ctx); err != nil {
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

// syncWarpRoutingConfig registers the warp-routing configuration from Tunnel/ClusterTunnel spec
// to ConfigMap. The TunnelConfig controller watches ConfigMaps and syncs to Cloudflare API.
//
// The Tunnel/ClusterTunnel controller is the ONLY source of truth for:
// - warp-routing configuration
// - fallback target
// - global origin request settings
//
// Other controllers (Ingress, Gateway, TunnelBinding) only contribute ingress rules.
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

	// Determine tunnel kind for source identification
	tunnelKind := kindTunnel
	if r.GetTunnel().GetNamespace() == "" {
		tunnelKind = kindClusterTunnel
	}

	r.GetLog().Info("Registering tunnel settings to ConfigMap",
		"tunnelId", tunnelID,
		"kind", tunnelKind,
		"name", r.GetTunnel().GetName(),
		"enableWarpRouting", enableWarpRouting,
		"fallbackTarget", fallbackTarget)

	// Build credentials reference from Tunnel spec
	credRef := getCredentialsReference(r)

	// Write to ConfigMap
	if err := writeTunnelSettingsToConfigMap(r, tunnelID, tunnelKind, enableWarpRouting, credRef); err != nil {
		return fmt.Errorf("failed to write tunnel settings to ConfigMap: %w", err)
	}

	r.GetLog().Info("Successfully registered tunnel settings to ConfigMap",
		"tunnelId", tunnelID,
		"kind", tunnelKind,
		"name", r.GetTunnel().GetName())
	r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeNormal,
		"SettingsRegistered", fmt.Sprintf("Tunnel settings registered: warp-routing=%v, fallback=%s", enableWarpRouting, fallbackTarget))

	return nil
}

// writeTunnelSettingsToConfigMap writes tunnel settings to the ConfigMap for the new architecture.
func writeTunnelSettingsToConfigMap(r GenericTunnelReconciler, tunnelID, tunnelKind string, enableWarpRouting bool, credRef v1alpha2.CredentialsReference) error {
	tunnel := r.GetTunnel()

	// Build tunnel settings for ConfigMap
	settings := &tunnelconfig.TunnelSettings{
		WARPRouting: enableWarpRouting,
	}

	// Build source config
	source := &tunnelconfig.SourceConfig{
		Kind:       tunnelKind,
		Namespace:  tunnel.GetNamespace(),
		Name:       tunnel.GetName(),
		Generation: tunnel.GetObject().GetGeneration(),
		Settings:   settings,
	}

	// Get owner GVK
	ownerGVK := metav1.GroupVersionKind{
		Group:   "networking.cloudflare-operator.io",
		Version: "v1alpha2",
		Kind:    tunnelKind,
	}

	// Build credentials ref for ConfigMap
	var configCredRef *tunnelconfig.CredentialsRef
	if credRef.Name != "" {
		configCredRef = &tunnelconfig.CredentialsRef{Name: credRef.Name}
	}

	// Get operator namespace from tunnel namespace (for namespaced Tunnel) or use default
	operatorNamespace := tunnel.GetNamespace()
	if operatorNamespace == "" {
		// ClusterTunnel - get from context or use default
		operatorNamespace = "cloudflare-operator-system"
	}

	// Write to ConfigMap
	writer := tunnelconfig.NewWriter(r.GetClient(), operatorNamespace)
	if err := writer.SetTunnelSettings(
		r.GetContext(),
		tunnelID,
		r.GetCfAPI().ValidAccountId,
		tunnel.GetStatus().TunnelName,
		settings,
		configCredRef,
		source,
		tunnel.GetObject(),
		ownerGVK,
	); err != nil {
		return fmt.Errorf("failed to write tunnel settings to ConfigMap: %w", err)
	}

	r.GetLog().V(1).Info("Wrote tunnel settings to ConfigMap",
		"tunnelId", tunnelID,
		"source", fmt.Sprintf("%s/%s/%s", tunnelKind, tunnel.GetNamespace(), tunnel.GetName()))

	return nil
}

// getCredentialsReference extracts the CredentialsReference from Tunnel spec.
// If credentialsRef is specified, use it; otherwise return an empty reference.
func getCredentialsReference(r GenericTunnelReconciler) v1alpha2.CredentialsReference {
	spec := r.GetTunnel().GetSpec()
	if spec.Cloudflare.CredentialsRef != nil {
		return v1alpha2.CredentialsReference{
			Name: spec.Cloudflare.CredentialsRef.Name,
		}
	}
	// Legacy mode: no CredentialsRef, return empty (sync controller will need to handle this)
	return v1alpha2.CredentialsReference{}
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
	//
	// Following six-layer architecture: Token is retrieved from:
	// 1. Annotation (set by applyLifecycleResult from SyncState)
	// 2. Existing token Secret (for re-reconciles)
	// Resource Controller MUST NOT call Cloudflare API directly
	token, err := getTunnelToken(r)
	if err != nil {
		r.GetLog().Error(err, "failed to get tunnel token")
		r.GetRecorder().Event(r.GetTunnel().GetObject(), corev1.EventTypeWarning, "FailedGetToken", "Failed to get tunnel token")
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

// getTunnelToken retrieves the tunnel token from available sources.
// Following six-layer architecture, this does NOT call Cloudflare API directly.
// Token sources (in order of precedence):
// 1. Annotation cloudflare-operator.io/tunnel-token (set by lifecycle result)
// 2. Existing token Secret (for re-reconciles after restart)
// 3. SyncState lifecycle result (if still available)
//
//nolint:revive // cyclomatic complexity is acceptable for multi-source lookup logic
func getTunnelToken(r GenericTunnelReconciler) (string, error) {
	ctx := r.GetContext()
	log := r.GetLog()
	tunnel := r.GetTunnel()

	// Source 1: Check annotation (set by applyLifecycleResult)
	if annotations := tunnel.GetAnnotations(); annotations != nil {
		if token, ok := annotations["cloudflare-operator.io/tunnel-token"]; ok && token != "" {
			log.V(1).Info("Using tunnel token from annotation")
			return token, nil
		}
	}

	// Source 2: Check existing token Secret
	tokenSecretName := tunnel.GetName() + "-token"
	tokenSecret := &corev1.Secret{}
	if err := r.GetClient().Get(ctx, apitypes.NamespacedName{
		Name:      tokenSecretName,
		Namespace: tunnel.GetNamespace(),
	}, tokenSecret); err == nil {
		if token, ok := tokenSecret.Data["token"]; ok && len(token) > 0 {
			log.V(1).Info("Using tunnel token from existing Secret")
			return string(token), nil
		}
	}

	// Source 3: Check SyncState lifecycle result
	tunnelName := ""
	if tunnel.GetSpec().NewTunnel != nil {
		tunnelName = tunnel.GetSpec().NewTunnel.Name
	} else if tunnel.GetSpec().ExistingTunnel != nil {
		tunnelName = tunnel.GetSpec().ExistingTunnel.Name
	}

	if tunnelName != "" {
		lifecycleSvc := tunnelsvc.NewLifecycleService(r.GetClient())
		result, err := lifecycleSvc.GetLifecycleResult(ctx, tunnelName)
		if err == nil && result != nil && result.TunnelToken != "" {
			log.V(1).Info("Using tunnel token from SyncState lifecycle result")
			// Store in annotation for future use
			tunnel.SetAnnotations(mergeAnnotations(tunnel.GetAnnotations(), map[string]string{
				"cloudflare-operator.io/tunnel-token": result.TunnelToken,
			}))
			if updateErr := r.GetClient().Update(ctx, tunnel.GetObject()); updateErr != nil {
				log.V(1).Info("Failed to store token in annotation, continuing", "error", updateErr)
			}
			return result.TunnelToken, nil
		}
	}

	return "", fmt.Errorf("tunnel token not found: check lifecycle SyncState or annotation")
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
	_ = ctrl.SetControllerReference(r.GetTunnel().GetObject(), sec, r.GetScheme())
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
	_ = ctrl.SetControllerReference(r.GetTunnel().GetObject(), dep, r.GetScheme())
	return dep
}
