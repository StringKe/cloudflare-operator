// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package origincacertificate provides a controller for managing Cloudflare Origin CA certificates.
package origincacertificate

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/controller"
	"github.com/StringKe/cloudflare-operator/internal/service"
	domainsvc "github.com/StringKe/cloudflare-operator/internal/service/domain"
)

const (
	finalizerName = "cloudflare.com/origin-ca-certificate-finalizer"
)

var (
	errPrivateKeyNotFound = errors.New("private key not found in secret")
	// errLifecyclePending is returned when a lifecycle operation is pending completion.
	// This triggers a requeue to check the operation status later.
	errLifecyclePending = errors.New("lifecycle operation pending, waiting for completion")
)

// Reconciler reconciles an OriginCACertificate object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Service  *domainsvc.OriginCACertificateService // Core Service for SyncState management

	// Internal state
	ctx  context.Context
	log  logr.Logger
	cert *networkingv1alpha2.OriginCACertificate
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles OriginCACertificate reconciliation
//
//nolint:revive // cognitive complexity is acceptable for main reconcile loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.ctx = ctx
	r.log = ctrl.LoggerFrom(ctx)

	// Get the OriginCACertificate resource
	r.cert = &networkingv1alpha2.OriginCACertificate{}
	if err := r.Get(ctx, req.NamespacedName, r.cert); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "unable to fetch OriginCACertificate")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !r.cert.DeletionTimestamp.IsZero() {
		return r.handleDeletion()
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(r.cert, finalizerName) {
		controllerutil.AddFinalizer(r.cert, finalizerName)
		if err := r.Update(ctx, r.cert); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if there's a pending lifecycle operation result to apply
	if err := r.applyLifecycleResult(); err != nil {
		if errors.Is(err, errLifecyclePending) {
			// Operation still pending, requeue to check later
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		r.log.Error(err, "Failed to apply lifecycle result")
	}

	// Check if certificate already exists
	if r.cert.Status.CertificateID != "" {
		return r.reconcileExisting()
	}

	// Issue new certificate via SyncState
	return r.issueCertificate()
}

// handleDeletion handles the deletion of OriginCACertificate
//
//nolint:revive // cognitive complexity is acceptable for deletion handling
func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.cert, finalizerName) {
		return ctrl.Result{}, nil
	}

	// Revoke certificate via SyncState if it exists
	if r.cert.Status.CertificateID != "" && r.Service != nil {
		// Build credentials reference
		credRef := r.buildCredentialsRef()

		// Request revocation via Service
		source := service.Source{
			Kind:      "OriginCACertificate",
			Namespace: r.cert.Namespace,
			Name:      r.cert.Name,
		}

		_, err := r.Service.RequestRevoke(r.ctx, domainsvc.OriginCACertificateRevokeOptions{
			Source:         source,
			CredentialsRef: credRef,
			CertificateID:  r.cert.Status.CertificateID,
		})
		if err != nil {
			r.log.Error(err, "Failed to request certificate revocation, continuing with deletion")
		} else {
			r.log.Info("Certificate revocation requested", "certificateId", r.cert.Status.CertificateID)
		}

		// Cleanup SyncState
		if cleanupErr := r.Service.CleanupSyncState(r.ctx, r.cert.Namespace, r.cert.Name); cleanupErr != nil {
			r.log.Error(cleanupErr, "Failed to cleanup SyncState, continuing with deletion")
		}
	}

	// Delete synced Secret if it exists
	if r.cert.Status.SecretName != "" {
		secret := &corev1.Secret{}
		secretNS := r.cert.Status.SecretNamespace
		if secretNS == "" {
			secretNS = r.cert.Namespace
		}
		if err := r.Get(r.ctx, types.NamespacedName{
			Name:      r.cert.Status.SecretName,
			Namespace: secretNS,
		}, secret); err == nil {
			// Check if we own this Secret
			if metav1.IsControlledBy(secret, r.cert) {
				if err := r.Delete(r.ctx, secret); err != nil && !apierrors.IsNotFound(err) {
					r.log.Error(err, "Failed to delete synced Secret")
				}
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(r.ctx, r.Client, r.cert, func() {
		controllerutil.RemoveFinalizer(r.cert, finalizerName)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileExisting handles an existing certificate
func (r *Reconciler) reconcileExisting() (ctrl.Result, error) {
	// Certificate already exists - check if renewal is needed
	if r.shouldRenew() {
		r.log.Info("Certificate needs renewal", "expiresAt", r.cert.Status.ExpiresAt)
		return r.renewCertificate()
	}

	// Sync to Secret if configured
	if err := r.syncSecret(); err != nil {
		r.log.Error(err, "Failed to sync Secret")
		r.Recorder.Event(r.cert, corev1.EventTypeWarning, "SecretSyncFailed", err.Error())
	}

	// Update state to Ready
	r.updateState(networkingv1alpha2.OriginCACertificateStateReady, "Certificate is ready")

	// Calculate next reconcile time
	requeueAfter := r.calculateRequeueTime()
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// issueCertificate issues a new certificate via SyncState
//
//nolint:revive // cognitive complexity is acceptable for certificate issuance
func (r *Reconciler) issueCertificate() (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.OriginCACertificateStateIssuing, "Issuing certificate")

	// Get or generate CSR
	csr, privateKey, err := r.getOrGenerateCSR()
	if err != nil {
		r.updateState(networkingv1alpha2.OriginCACertificateStateError, fmt.Sprintf("Failed to generate CSR: %v", err))
		r.Recorder.Event(r.cert, corev1.EventTypeWarning, "CSRGenerationFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Store private key in annotation temporarily for later Secret sync
	if len(privateKey) > 0 {
		if r.cert.Annotations == nil {
			r.cert.Annotations = make(map[string]string)
		}
		r.cert.Annotations["cloudflare-operator.io/private-key"] = string(privateKey)
		if err := r.Update(r.ctx, r.cert); err != nil {
			r.log.Error(err, "Failed to store private key annotation")
		}
	}

	// Build credentials reference
	credRef := r.buildCredentialsRef()

	// Calculate validity
	validity := int(r.cert.Spec.Validity)
	if validity == 0 {
		validity = 5475 // Default 15 years
	}

	requestType := string(r.cert.Spec.RequestType)
	if requestType == "" {
		requestType = "origin-rsa"
	}

	// Request certificate creation via Service
	source := service.Source{
		Kind:      "OriginCACertificate",
		Namespace: r.cert.Namespace,
		Name:      r.cert.Name,
	}

	_, err = r.Service.RequestCreate(r.ctx, domainsvc.OriginCACertificateCreateOptions{
		Source:         source,
		CredentialsRef: credRef,
		Hostnames:      r.cert.Spec.Hostnames,
		RequestType:    requestType,
		ValidityDays:   validity,
		CSR:            csr,
	})
	if err != nil {
		r.updateState(networkingv1alpha2.OriginCACertificateStateError, fmt.Sprintf("Failed to request certificate: %v", err))
		r.Recorder.Event(r.cert, corev1.EventTypeWarning, "RequestFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	r.log.Info("Certificate creation requested, waiting for completion")
	r.Recorder.Event(r.cert, corev1.EventTypeNormal, "CertificateRequested",
		"Certificate creation requested via SyncState")

	// Requeue to check for completion
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// renewCertificate renews the certificate via SyncState
//
//nolint:revive // cognitive complexity is acceptable for renewal logic
func (r *Reconciler) renewCertificate() (ctrl.Result, error) {
	r.updateState(networkingv1alpha2.OriginCACertificateStateRenewing, "Renewing certificate")

	// Get or generate CSR
	csr, privateKey, err := r.getOrGenerateCSR()
	if err != nil {
		r.updateState(networkingv1alpha2.OriginCACertificateStateError, fmt.Sprintf("Failed to generate CSR: %v", err))
		r.Recorder.Event(r.cert, corev1.EventTypeWarning, "CSRGenerationFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Store private key in annotation temporarily for later Secret sync
	if len(privateKey) > 0 {
		if r.cert.Annotations == nil {
			r.cert.Annotations = make(map[string]string)
		}
		r.cert.Annotations["cloudflare-operator.io/private-key"] = string(privateKey)
		if err := r.Update(r.ctx, r.cert); err != nil {
			r.log.Error(err, "Failed to store private key annotation")
		}
	}

	// Build credentials reference
	credRef := r.buildCredentialsRef()

	// Calculate validity
	validity := int(r.cert.Spec.Validity)
	if validity == 0 {
		validity = 5475 // Default 15 years
	}

	requestType := string(r.cert.Spec.RequestType)
	if requestType == "" {
		requestType = "origin-rsa"
	}

	// Request certificate renewal via Service
	source := service.Source{
		Kind:      "OriginCACertificate",
		Namespace: r.cert.Namespace,
		Name:      r.cert.Name,
	}

	oldCertID := r.cert.Status.CertificateID

	_, err = r.Service.RequestRenew(r.ctx, domainsvc.OriginCACertificateRenewOptions{
		Source:         source,
		CredentialsRef: credRef,
		CertificateID:  oldCertID,
		Hostnames:      r.cert.Spec.Hostnames,
		RequestType:    requestType,
		ValidityDays:   validity,
		CSR:            csr,
	})
	if err != nil {
		r.updateState(networkingv1alpha2.OriginCACertificateStateError, fmt.Sprintf("Failed to request renewal: %v", err))
		r.Recorder.Event(r.cert, corev1.EventTypeWarning, "RenewalRequestFailed", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	r.log.Info("Certificate renewal requested, waiting for completion", "oldCertId", oldCertID)
	r.Recorder.Event(r.cert, corev1.EventTypeNormal, "RenewalRequested",
		"Certificate renewal requested via SyncState")

	// Clear old certificate ID to indicate renewal is in progress
	r.cert.Status.CertificateID = ""
	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.cert, func() {
		r.cert.Status.CertificateID = ""
	}); err != nil {
		r.log.Error(err, "Failed to clear certificate ID")
	}

	// Requeue to check for completion
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// shouldRenew checks if the certificate should be renewed
func (r *Reconciler) shouldRenew() bool {
	if r.cert.Spec.Renewal == nil || !r.cert.Spec.Renewal.Enabled {
		return false
	}

	if r.cert.Status.ExpiresAt == nil {
		return false
	}

	renewBeforeDays := r.cert.Spec.Renewal.RenewBeforeDays
	if renewBeforeDays == 0 {
		renewBeforeDays = 30
	}

	renewalThreshold := r.cert.Status.ExpiresAt.Time.AddDate(0, 0, -renewBeforeDays) //nolint:staticcheck // Time embedded access required
	return time.Now().After(renewalThreshold)
}

// calculateRequeueTime calculates when to next reconcile
func (r *Reconciler) calculateRequeueTime() time.Duration {
	// If renewal is configured, requeue at renewal time
	if r.cert.Status.RenewalTime != nil {
		timeUntilRenewal := time.Until(r.cert.Status.RenewalTime.Time)
		if timeUntilRenewal > 0 {
			// Add some jitter
			return timeUntilRenewal + time.Duration(time.Now().UnixNano()%int64(time.Hour))
		}
	}

	// Default: check daily
	return 24 * time.Hour
}

// getOrGenerateCSR gets an existing CSR or generates a new one
//
//nolint:revive // cyclomatic complexity is acceptable for CSR generation logic
func (r *Reconciler) getOrGenerateCSR() (string, []byte, error) {
	// If CSR is provided in spec, use it
	if r.cert.Spec.CSR != "" {
		// Try to get private key from secret
		privateKey, err := r.getPrivateKeyFromSecret()
		if err != nil {
			return "", nil, fmt.Errorf("CSR provided but no private key found: %w", err)
		}
		return r.cert.Spec.CSR, privateKey, nil
	}

	// Generate new key pair and CSR
	algorithm := "RSA"
	keySize := 2048
	if r.cert.Spec.PrivateKey != nil {
		if r.cert.Spec.PrivateKey.Algorithm != "" {
			algorithm = r.cert.Spec.PrivateKey.Algorithm
		}
		if r.cert.Spec.PrivateKey.Size != 0 {
			keySize = r.cert.Spec.PrivateKey.Size
		}
	}

	var privateKey any
	var privateKeyPEM []byte
	var err error

	switch algorithm {
	case "RSA":
		key, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			return "", nil, fmt.Errorf("failed to generate RSA key: %w", err)
		}
		privateKey = key
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
	case "ECDSA":
		var curve elliptic.Curve
		switch keySize {
		case 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		default:
			curve = elliptic.P256()
		}
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return "", nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
		}
		privateKey = key
		keyBytes, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal ECDSA key: %w", err)
		}
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyBytes,
		})
	default:
		return "", nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	// Create CSR
	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: r.cert.Spec.Hostnames[0],
		},
		DNSNames: r.cert.Spec.Hostnames,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privateKeyPEM, nil
}

// getPrivateKeyFromSecret retrieves the private key from a Secret
//
//nolint:revive // cognitive complexity is acceptable for key retrieval logic
func (r *Reconciler) getPrivateKeyFromSecret() ([]byte, error) {
	if r.cert.Spec.PrivateKey == nil || r.cert.Spec.PrivateKey.SecretRef == nil {
		return nil, errPrivateKeyNotFound
	}

	secretRef := r.cert.Spec.PrivateKey.SecretRef
	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = r.cert.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(r.ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get private key secret: %w", err)
	}

	key := secretRef.Key
	if key == "" {
		key = "tls.key"
	}

	privateKey, ok := secret.Data[key]
	if !ok {
		// Try alternative key name
		privateKey, ok = secret.Data["private-key"]
		if !ok {
			return nil, errPrivateKeyNotFound
		}
	}

	return privateKey, nil
}

// syncSecret syncs the certificate to a Kubernetes Secret
func (r *Reconciler) syncSecret() error {
	// Get private key from existing secret
	privateKey, err := r.getPrivateKeyFromSyncedSecret()
	if err != nil {
		// If we don't have the private key, we can't sync
		r.log.V(1).Info("Private key not available for sync", "error", err)
		return nil
	}
	return r.syncSecretWithKey(privateKey)
}

// getPrivateKeyFromSyncedSecret retrieves the private key from the synced secret
func (r *Reconciler) getPrivateKeyFromSyncedSecret() ([]byte, error) {
	if r.cert.Status.SecretName == "" {
		return nil, errPrivateKeyNotFound
	}

	namespace := r.cert.Status.SecretNamespace
	if namespace == "" {
		namespace = r.cert.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(r.ctx, types.NamespacedName{
		Name:      r.cert.Status.SecretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, err
	}

	// Try different key names
	for _, key := range []string{"tls.key", "private-key"} {
		if data, ok := secret.Data[key]; ok {
			return data, nil
		}
	}

	return nil, errPrivateKeyNotFound
}

// syncSecretWithKey syncs the certificate and key to a Kubernetes Secret
//
//nolint:revive // cyclomatic complexity is acceptable for secret sync logic
func (r *Reconciler) syncSecretWithKey(privateKey []byte) error {
	syncConfig := r.cert.Spec.SecretSync
	if syncConfig == nil || !syncConfig.Enabled {
		return nil
	}

	secretName := syncConfig.SecretName
	if secretName == "" {
		secretName = r.cert.Name
	}

	namespace := syncConfig.Namespace
	if namespace == "" {
		namespace = r.cert.Namespace
	}

	// Create or update Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, secret, func() error {
		// Set owner reference only if in same namespace
		if namespace == r.cert.Namespace {
			if err := controllerutil.SetControllerReference(r.cert, secret, r.Scheme); err != nil {
				return err
			}
		}

		// Set labels
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "cloudflare-operator"
		secret.Labels["cloudflare-operator.io/origin-ca-certificate"] = r.cert.Name

		// Set data based on format
		if syncConfig.CertManagerCompatible {
			secret.Type = corev1.SecretTypeTLS
			secret.Data = map[string][]byte{
				"tls.crt": []byte(r.cert.Status.Certificate),
				"tls.key": privateKey,
			}
			if syncConfig.IncludeCA {
				secret.Data["ca.crt"] = []byte(cloudflareOriginCARoot)
			}
		} else {
			secret.Type = corev1.SecretTypeOpaque
			secret.Data = map[string][]byte{
				"certificate": []byte(r.cert.Status.Certificate),
				"private-key": privateKey,
			}
			if syncConfig.IncludeCA {
				secret.Data["ca-certificate"] = []byte(cloudflareOriginCARoot)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to sync secret: %w", err)
	}

	r.log.Info("Secret synced", "operation", op, "secretName", secretName, "namespace", namespace)

	// Update status
	r.cert.Status.SecretName = secretName
	r.cert.Status.SecretNamespace = namespace

	return nil
}

// applyLifecycleResult checks for completed lifecycle operation and applies the result
//
//nolint:revive // cognitive complexity is acceptable for result application logic
func (r *Reconciler) applyLifecycleResult() error {
	if r.Service == nil {
		return nil
	}

	// Check if there's a completed lifecycle operation
	result, err := r.Service.GetLifecycleResult(r.ctx, r.cert.Namespace, r.cert.Name)
	if err != nil {
		return fmt.Errorf("get lifecycle result: %w", err)
	}

	if result == nil {
		// Check if operation is still pending
		completed, err := r.Service.IsLifecycleCompleted(r.ctx, r.cert.Namespace, r.cert.Name)
		if err != nil {
			return err
		}
		if !completed {
			// Check for errors
			errMsg, _ := r.Service.GetLifecycleError(r.ctx, r.cert.Namespace, r.cert.Name)
			if errMsg != "" {
				return fmt.Errorf("lifecycle operation failed: %s", errMsg)
			}
			return errLifecyclePending
		}
		return nil
	}

	// Apply result to status
	now := metav1.Now()
	r.cert.Status.CertificateID = result.CertificateID
	r.cert.Status.Certificate = result.Certificate
	r.cert.Status.IssuedAt = &now
	if result.ExpiresAt != nil {
		r.cert.Status.ExpiresAt = result.ExpiresAt
	}

	// Calculate renewal time
	if r.cert.Spec.Renewal != nil && r.cert.Spec.Renewal.Enabled && r.cert.Status.ExpiresAt != nil {
		renewBeforeDays := r.cert.Spec.Renewal.RenewBeforeDays
		if renewBeforeDays == 0 {
			renewBeforeDays = 30
		}
		renewalTime := r.cert.Status.ExpiresAt.Time.AddDate(0, 0, -renewBeforeDays) //nolint:staticcheck // Time embedded access required
		renewalTimeMeta := metav1.NewTime(renewalTime)
		r.cert.Status.RenewalTime = &renewalTimeMeta
	}

	// Sync to Secret if configured
	if privateKeyStr, ok := r.cert.Annotations["cloudflare-operator.io/private-key"]; ok && privateKeyStr != "" {
		if err := r.syncSecretWithKey([]byte(privateKeyStr)); err != nil {
			r.log.Error(err, "Failed to sync Secret")
			r.Recorder.Event(r.cert, corev1.EventTypeWarning, "SecretSyncFailed", err.Error())
		}

		// Remove the private key from annotation after syncing
		delete(r.cert.Annotations, "cloudflare-operator.io/private-key")
		if updateErr := r.Update(r.ctx, r.cert); updateErr != nil {
			r.log.Error(updateErr, "Failed to remove private key annotation")
		}
	}

	r.log.Info("Lifecycle result applied",
		"certificateId", result.CertificateID,
		"expiresAt", r.cert.Status.ExpiresAt)
	r.Recorder.Event(r.cert, corev1.EventTypeNormal, "CertificateIssued",
		fmt.Sprintf("Certificate issued with ID %s", result.CertificateID))

	return nil
}

// buildCredentialsRef builds a CredentialsReference from spec
func (r *Reconciler) buildCredentialsRef() networkingv1alpha2.CredentialsReference {
	if r.cert.Spec.CredentialsRef != nil {
		return networkingv1alpha2.CredentialsReference{
			Name: r.cert.Spec.CredentialsRef.Name,
		}
	}
	return networkingv1alpha2.CredentialsReference{
		Name: "default",
	}
}

// updateState updates the state and status of the OriginCACertificate
func (r *Reconciler) updateState(state networkingv1alpha2.OriginCACertificateState, message string) {
	r.cert.Status.State = state
	r.cert.Status.Message = message
	r.cert.Status.ObservedGeneration = r.cert.Generation

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: r.cert.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.OriginCACertificateStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "CertificateReady"
	}

	controller.SetCondition(&r.cert.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)

	if err := controller.UpdateStatusWithConflictRetry(r.ctx, r.Client, r.cert, func() {
		r.cert.Status.State = state
		r.cert.Status.Message = message
		r.cert.Status.ObservedGeneration = r.cert.Generation
		controller.SetCondition(&r.cert.Status.Conditions, condition.Type, condition.Status, condition.Reason, condition.Message)
	}); err != nil {
		r.log.Error(err, "failed to update status")
	}
}

// findCertificatesForCredentials returns OriginCACertificates that reference the given credentials
//
//nolint:revive // cognitive complexity is acceptable for watch handler
func (r *Reconciler) findCertificatesForCredentials(ctx context.Context, obj client.Object) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}

	certList := &networkingv1alpha2.OriginCACertificateList{}
	if err := r.List(ctx, certList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, cert := range certList.Items {
		if cert.Spec.CredentialsRef != nil && cert.Spec.CredentialsRef.Name == creds.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cert.Name,
					Namespace: cert.Namespace,
				},
			})
		}

		if creds.Spec.IsDefault && cert.Spec.CredentialsRef == nil {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cert.Name,
					Namespace: cert.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.OriginCACertificate{}).
		Owns(&corev1.Secret{}).
		Watches(&networkingv1alpha2.CloudflareCredentials{},
			handler.EnqueueRequestsFromMapFunc(r.findCertificatesForCredentials)).
		Named("origincacertificate").
		Complete(r)
}

// Cloudflare Origin CA root certificate
// This is the root certificate that signs all Origin CA certificates
// See: https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/#cloudflare-origin-ca-root-certificate
const cloudflareOriginCARoot = `-----BEGIN CERTIFICATE-----
MIIGCjCCA/KgAwIBAgIIV5G6lVbCLmEwDQYJKoZIhvcNAQENBQAwgZAxCzAJBgNV
BAYTAlVTMRkwFwYDVQQKExBDbG91ZEZsYXJlLCBJbmMuMRQwEgYDVQQLEwtPcmln
aW4gUHVsbDEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzETMBEGA1UECBMKQ2FsaWZv
cm5pYTEjMCEGA1UEAxMab3JpZ2luLXB1bGwuY2xvdWRmbGFyZS5uZXQwHhcNMTkx
MDEwMTg0NTAwWhcNMjkxMTAxMTcwMDAwWjCBkDELMAkGA1UEBhMCVVMxGTAXBgNV
BAoTEENsb3VkRmxhcmUsIEluYy4xFDASBgNVBAsTC09yaWdpbiBQdWxsMRYwFAYD
VQQHEw1TYW4gRnJhbmNpc2NvMRMwEQYDVQQIEwpDYWxpZm9ybmlhMSMwIQYDVQQD
ExpvcmlnaW4tcHVsbC5jbG91ZGZsYXJlLm5ldDCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBAN2y2zojYfl0bKfhp0AJBFeV+jQqbCw3sHmvEPwLmqDLqynI
42tZXR5y914ZB9ZrwbL/K5O46exd/LujJnV2b3dzcx5rtiQzso0xzljqbnbQT20e
ihx/WrF4OkZKTSPIyTsRiHTxJtFlpgRJII5RpSKV1m3EZ37l5bB36Tvkzf3CwHcN
j7kDtOzh8E4RWBpNjLwqHxHxQxKkKKnFoKCiI2C0hQKNwdT0hlLsq5x2k9foDuwT
lJuOvXpyzzh6jCDMDwfQVwHEZxMUOdPrOODPwvFApdwJJlJTv2k7tPKGLPLQIVVb
N/gRRES4RWTBynFjMKfMl85olJr0LMNsmwYx7E9dBfP+xVUtZNfwk7J8JodWL0U2
f8m3FoSB/N6sS5gBPo2pZJHNvJ8CMNj+kGfO+VN5TjJAnfKlJt+c/Vs1FElLNeKu
odACxWzDPsr+FdeZ8AvJoKZjMz8fpNJr6fMEafj/TP/jR/u1HEtB4sO3SzKmlJSn
cDjEjVPQH1Cj7qPzJvA1QDvxY7Z+A3q5J/H0yzJkPjlwqf/lOaQRfJNnCMUL89+v
RQdxFKbsZ0LFy/S3h7T5mhjFIe9A5kNZE+5z1yOSqE9B6c1l7ib1KxlPLgFvCkPO
pj+B8G+pFrg/Wz/wADQrT4p2GH0pJcKU0lCNc+1BAhxNxfVO0LHzdiUoXS8FAgMB
AAGjZjBkMA4GA1UdDwEB/wQEAwIBBjASBgNVHRMBAf8ECDAGAQH/AgECMB0GA1Ud
DgQWBBRDWUsraYuA4REzalfNVzjann3F6zAfBgNVHSMEGDAWgBRDWUsraYuA4REz
alfNVzjann3F6zANBgkqhkiG9w0BAQ0FAAOCAgEAkQ+T9nqcSlAuW/90DeYmQOW1
QhqOor5psBEGvxbNGV2hdLJY8h6QUq48BCevcMChg/L1CkznBNI40i3/6heDn3IS
zVEwXKf34pPFCACWVMZxbQjkNRTiH8iRur9EsaNQ5oXCPJkhwg2+IFyoPAAYURoX
VcI9SCDUa45clmYHJ/XYwV1icGVI8/9b2JUqklnOTa5tugwIUi5sTfipNcJXHhgz
6BKYDl0/UP0leBSAGmq3RfVUJ4SQp5qZ5u4PTv8KJ0BNpP7K8MwpzW7P5r7mPFCI
YCi8EZul5dlCqxy0aEEnDn+qk2RxPzVHyqKXaPSmKPnNxMhptG6uCnJZ+aIwzfqw
Rw4xjAd4TLNmlSFe9oL7Ket6b7GuNgvhMYfVsHssLcH6NES7Fwi7C0lxMIV2Llgu
X2ur3Xzng6WExqwR/IvWMu6pAYfjBXAaLR5BxMjN2Opa1rcS4w6HnAp+INz3gO0P
SjmL/5mkhQhQ0vt2MszMl4H/q9VqgCFhB33pMxPKrq4q1GqLJEnRZe6M1Mept6FZ
4O4hBSZzMAVLbpCxDaUClB7QEZ+5P8E5t/3cALG9q/7A/z7ZfNx2ZMdMnBfAFeMS
Xq3Y7kMOCiS+C/2rGXQmjl4oeq3aXwMoq7G/TjDqjCc9wK+s9FjL0a9Q8yqdRRlz
bVWvBqPrOGi69PpfyA0=
-----END CERTIFICATE-----`
