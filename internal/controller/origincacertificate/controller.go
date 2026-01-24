// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package origincacertificate provides a controller for managing Cloudflare Origin CA certificates.
// It directly calls Cloudflare API and writes status back to the CRD.
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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
)

const (
	finalizerName = "cloudflare.com/origin-ca-certificate-finalizer"
)

var (
	errPrivateKeyNotFound = errors.New("private key not found in secret")
)

// Reconciler reconciles an OriginCACertificate object.
// It directly calls Cloudflare API and writes status back to the CRD.
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	APIFactory *common.APIClientFactory
}

// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.cloudflare-operator.io,resources=origincacertificates/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles OriginCACertificate reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the OriginCACertificate resource
	cert := &networkingv1alpha2.OriginCACertificate{}
	if err := r.Get(ctx, req.NamespacedName, cert); err != nil {
		if apierrors.IsNotFound(err) {
			return common.NoRequeue(), nil
		}
		logger.Error(err, "Unable to fetch OriginCACertificate")
		return common.NoRequeue(), err
	}

	// Handle deletion
	if !cert.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, cert)
	}

	// Ensure finalizer
	if added, err := controller.EnsureFinalizer(ctx, r.Client, cert, finalizerName); err != nil {
		return common.NoRequeue(), err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if certificate already exists
	if cert.Status.CertificateID != "" {
		return r.reconcileExisting(ctx, cert)
	}

	// Issue new certificate
	return r.issueCertificate(ctx, cert)
}

// handleDeletion handles the deletion of OriginCACertificate.
func (r *Reconciler) handleDeletion(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cert, finalizerName) {
		return common.NoRequeue(), nil
	}

	// Revoke certificate if it exists
	if cert.Status.CertificateID != "" {
		apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
			CredentialsRef: r.buildCredentialsRef(cert),
			Namespace:      cert.Namespace,
		})
		if err != nil {
			logger.Error(err, "Failed to get API client for deletion")
			// Continue with finalizer removal
		} else {
			logger.Info("Revoking Origin CA certificate", "certificateId", cert.Status.CertificateID)
			if err := apiResult.API.RevokeOriginCACertificate(ctx, cert.Status.CertificateID); err != nil {
				if !cf.IsNotFoundError(err) {
					logger.Error(err, "Failed to revoke certificate")
					r.Recorder.Event(cert, corev1.EventTypeWarning, "RevokeFailed",
						fmt.Sprintf("Failed to revoke certificate: %s", cf.SanitizeErrorMessage(err)))
					// Continue with deletion anyway
				}
			} else {
				r.Recorder.Event(cert, corev1.EventTypeNormal, "Revoked",
					"Origin CA certificate revoked from Cloudflare")
			}
		}
	}

	// Delete synced Secret if it exists
	if cert.Status.SecretName != "" {
		secret := &corev1.Secret{}
		secretNS := cert.Status.SecretNamespace
		if secretNS == "" {
			secretNS = cert.Namespace
		}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      cert.Status.SecretName,
			Namespace: secretNS,
		}, secret); err == nil {
			// Check if we own this Secret
			if metav1.IsControlledBy(secret, cert) {
				if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed to delete synced Secret")
				}
			}
		}
	}

	// Remove finalizer
	if err := controller.UpdateWithConflictRetry(ctx, r.Client, cert, func() {
		controllerutil.RemoveFinalizer(cert, finalizerName)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return common.NoRequeue(), err
	}
	r.Recorder.Event(cert, corev1.EventTypeNormal, controller.EventReasonFinalizerRemoved, "Finalizer removed")

	return common.NoRequeue(), nil
}

// reconcileExisting handles an existing certificate.
func (r *Reconciler) reconcileExisting(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Certificate already exists - check if renewal is needed
	if r.shouldRenew(cert) {
		logger.Info("Certificate needs renewal", "expiresAt", cert.Status.ExpiresAt)
		return r.renewCertificate(ctx, cert)
	}

	// Sync to Secret if configured
	if err := r.syncSecret(ctx, cert); err != nil {
		logger.Error(err, "Failed to sync Secret")
		r.Recorder.Event(cert, corev1.EventTypeWarning, "SecretSyncFailed", err.Error())
	}

	// Update state to Ready
	return r.updateStatusReady(ctx, cert)
}

// issueCertificate issues a new certificate directly via Cloudflare API.
//
//nolint:revive // cognitive complexity is acceptable for certificate issuance
func (r *Reconciler) issueCertificate(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.updateState(ctx, cert, networkingv1alpha2.OriginCACertificateStateIssuing, "Issuing certificate")

	// Get or generate CSR
	csr, privateKey, err := r.getOrGenerateCSR(ctx, cert)
	if err != nil {
		r.updateState(ctx, cert, networkingv1alpha2.OriginCACertificateStateError,
			fmt.Sprintf("Failed to generate CSR: %v", err))
		r.Recorder.Event(cert, corev1.EventTypeWarning, "CSRGenerationFailed", err.Error())
		return common.RequeueShort(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: r.buildCredentialsRef(cert),
		Namespace:      cert.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, cert, err)
	}

	// Calculate validity
	validity := int(cert.Spec.Validity)
	if validity == 0 {
		validity = 5475 // Default 15 years
	}

	requestType := string(cert.Spec.RequestType)
	if requestType == "" {
		requestType = "origin-rsa"
	}

	// Create certificate via Cloudflare API
	logger.Info("Creating Origin CA certificate in Cloudflare",
		"hostnames", cert.Spec.Hostnames,
		"requestType", requestType,
		"validity", validity)

	result, err := apiResult.API.CreateOriginCACertificate(ctx, cf.OriginCACertificateParams{
		Hostnames:       cert.Spec.Hostnames,
		RequestType:     requestType,
		RequestValidity: validity,
		CSR:             csr,
	})
	if err != nil {
		logger.Error(err, "Failed to create Origin CA certificate")
		return r.updateStatusError(ctx, cert, err)
	}

	r.Recorder.Event(cert, corev1.EventTypeNormal, "Created",
		fmt.Sprintf("Origin CA certificate created with ID %s", result.ID))

	// Update status with certificate data
	return r.updateStatusWithCertificate(ctx, cert, result, privateKey)
}

// renewCertificate renews the certificate.
//
//nolint:revive // cognitive complexity is acceptable for renewal logic
func (r *Reconciler) renewCertificate(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.updateState(ctx, cert, networkingv1alpha2.OriginCACertificateStateRenewing, "Renewing certificate")

	// Get or generate CSR
	csr, privateKey, err := r.getOrGenerateCSR(ctx, cert)
	if err != nil {
		r.updateState(ctx, cert, networkingv1alpha2.OriginCACertificateStateError,
			fmt.Sprintf("Failed to generate CSR: %v", err))
		r.Recorder.Event(cert, corev1.EventTypeWarning, "CSRGenerationFailed", err.Error())
		return common.RequeueShort(), nil
	}

	// Get API client
	apiResult, err := r.APIFactory.GetClient(ctx, common.APIClientOptions{
		CredentialsRef: r.buildCredentialsRef(cert),
		Namespace:      cert.Namespace,
	})
	if err != nil {
		logger.Error(err, "Failed to get API client")
		return r.updateStatusError(ctx, cert, err)
	}

	// Revoke old certificate first
	oldCertID := cert.Status.CertificateID
	if oldCertID != "" {
		logger.Info("Revoking old certificate before renewal", "oldCertId", oldCertID)
		if err := apiResult.API.RevokeOriginCACertificate(ctx, oldCertID); err != nil {
			if !cf.IsNotFoundError(err) {
				logger.Error(err, "Failed to revoke old certificate, continuing with renewal")
			}
		}
	}

	// Calculate validity
	validity := int(cert.Spec.Validity)
	if validity == 0 {
		validity = 5475 // Default 15 years
	}

	requestType := string(cert.Spec.RequestType)
	if requestType == "" {
		requestType = "origin-rsa"
	}

	// Create new certificate
	logger.Info("Creating renewed Origin CA certificate in Cloudflare",
		"hostnames", cert.Spec.Hostnames,
		"requestType", requestType,
		"validity", validity)

	result, err := apiResult.API.CreateOriginCACertificate(ctx, cf.OriginCACertificateParams{
		Hostnames:       cert.Spec.Hostnames,
		RequestType:     requestType,
		RequestValidity: validity,
		CSR:             csr,
	})
	if err != nil {
		logger.Error(err, "Failed to create renewed Origin CA certificate")
		return r.updateStatusError(ctx, cert, err)
	}

	r.Recorder.Event(cert, corev1.EventTypeNormal, "Renewed",
		fmt.Sprintf("Origin CA certificate renewed with new ID %s", result.ID))

	// Update status with new certificate data
	return r.updateStatusWithCertificate(ctx, cert, result, privateKey)
}

// shouldRenew checks if the certificate should be renewed.
func (*Reconciler) shouldRenew(cert *networkingv1alpha2.OriginCACertificate) bool {
	if cert.Spec.Renewal == nil || !cert.Spec.Renewal.Enabled {
		return false
	}

	if cert.Status.ExpiresAt == nil {
		return false
	}

	renewBeforeDays := cert.Spec.Renewal.RenewBeforeDays
	if renewBeforeDays == 0 {
		renewBeforeDays = 30
	}

	renewalThreshold := cert.Status.ExpiresAt.Time.AddDate(0, 0, -renewBeforeDays) //nolint:staticcheck // Time embedded access required
	return time.Now().After(renewalThreshold)
}

// calculateRequeueTime calculates when to next reconcile.
func (*Reconciler) calculateRequeueTime(cert *networkingv1alpha2.OriginCACertificate) time.Duration {
	// If renewal is configured, requeue at renewal time
	if cert.Status.RenewalTime != nil {
		timeUntilRenewal := time.Until(cert.Status.RenewalTime.Time)
		if timeUntilRenewal > 0 {
			// Add some jitter
			return timeUntilRenewal + time.Duration(time.Now().UnixNano()%int64(time.Hour))
		}
	}

	// Default: check daily
	return 24 * time.Hour
}

// getOrGenerateCSR gets an existing CSR or generates a new one.
//
//nolint:revive // cyclomatic complexity is acceptable for CSR generation logic
func (r *Reconciler) getOrGenerateCSR(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (string, []byte, error) {
	// If CSR is provided in spec, use it
	if cert.Spec.CSR != "" {
		// Try to get private key from secret
		privateKey, err := r.getPrivateKeyFromSecret(ctx, cert)
		if err != nil {
			return "", nil, fmt.Errorf("CSR provided but no private key found: %w", err)
		}
		return cert.Spec.CSR, privateKey, nil
	}

	// Generate new key pair and CSR
	algorithm := "RSA"
	keySize := 2048
	if cert.Spec.PrivateKey != nil {
		if cert.Spec.PrivateKey.Algorithm != "" {
			algorithm = cert.Spec.PrivateKey.Algorithm
		}
		if cert.Spec.PrivateKey.Size != 0 {
			keySize = cert.Spec.PrivateKey.Size
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
			CommonName: cert.Spec.Hostnames[0],
		},
		DNSNames: cert.Spec.Hostnames,
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

// getPrivateKeyFromSecret retrieves the private key from a Secret.
func (r *Reconciler) getPrivateKeyFromSecret(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) ([]byte, error) {
	if cert.Spec.PrivateKey == nil || cert.Spec.PrivateKey.SecretRef == nil {
		return nil, errPrivateKeyNotFound
	}

	secretRef := cert.Spec.PrivateKey.SecretRef
	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = cert.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
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

// syncSecret syncs the certificate to a Kubernetes Secret.
func (r *Reconciler) syncSecret(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) error {
	syncConfig := cert.Spec.SecretSync
	if syncConfig == nil || !syncConfig.Enabled {
		return nil
	}

	// Get private key from existing secret
	privateKey, err := r.getPrivateKeyFromSyncedSecret(ctx, cert)
	if err != nil {
		// If we don't have the private key, we can't sync
		return nil
	}

	return r.syncSecretWithKey(ctx, cert, privateKey)
}

// getPrivateKeyFromSyncedSecret retrieves the private key from the synced secret.
func (r *Reconciler) getPrivateKeyFromSyncedSecret(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) ([]byte, error) {
	if cert.Status.SecretName == "" {
		return nil, errPrivateKeyNotFound
	}

	namespace := cert.Status.SecretNamespace
	if namespace == "" {
		namespace = cert.Namespace
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cert.Status.SecretName,
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

// syncSecretWithKey syncs the certificate and key to a Kubernetes Secret.
//
//nolint:revive // cyclomatic complexity is acceptable for secret sync logic
func (r *Reconciler) syncSecretWithKey(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
	privateKey []byte,
) error {
	logger := log.FromContext(ctx)

	syncConfig := cert.Spec.SecretSync
	if syncConfig == nil || !syncConfig.Enabled {
		return nil
	}

	secretName := syncConfig.SecretName
	if secretName == "" {
		secretName = cert.Name
	}

	namespace := syncConfig.Namespace
	if namespace == "" {
		namespace = cert.Namespace
	}

	// Create or update Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set owner reference only if in same namespace
		if namespace == cert.Namespace {
			if err := controllerutil.SetControllerReference(cert, secret, r.Scheme); err != nil {
				return err
			}
		}

		// Set labels
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "cloudflare-operator"
		secret.Labels["cloudflare-operator.io/origin-ca-certificate"] = cert.Name

		// Set data based on format
		if syncConfig.CertManagerCompatible {
			secret.Type = corev1.SecretTypeTLS
			secret.Data = map[string][]byte{
				"tls.crt": []byte(cert.Status.Certificate),
				"tls.key": privateKey,
			}
			if syncConfig.IncludeCA {
				secret.Data["ca.crt"] = []byte(cloudflareOriginCARoot)
			}
		} else {
			secret.Type = corev1.SecretTypeOpaque
			secret.Data = map[string][]byte{
				"certificate": []byte(cert.Status.Certificate),
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

	logger.Info("Secret synced", "operation", op, "secretName", secretName, "namespace", namespace)

	// Update status with secret info (will be written in status update)
	cert.Status.SecretName = secretName
	cert.Status.SecretNamespace = namespace

	return nil
}

// buildCredentialsRef builds a CredentialsReference from spec.
func (*Reconciler) buildCredentialsRef(
	cert *networkingv1alpha2.OriginCACertificate,
) *networkingv1alpha2.CredentialsReference {
	if cert.Spec.CredentialsRef != nil {
		return &networkingv1alpha2.CredentialsReference{
			Name: cert.Spec.CredentialsRef.Name,
		}
	}
	return nil
}

// updateState updates the state and status of the OriginCACertificate.
func (r *Reconciler) updateState(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
	state networkingv1alpha2.OriginCACertificateState,
	message string,
) {
	logger := log.FromContext(ctx)

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cert.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(state),
		Message:            message,
	}

	if state == networkingv1alpha2.OriginCACertificateStateReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "CertificateReady"
	}

	if err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, cert, func() {
		cert.Status.State = state
		cert.Status.Message = message
		cert.Status.ObservedGeneration = cert.Generation
		meta.SetStatusCondition(&cert.Status.Conditions, condition)
	}); err != nil {
		logger.Error(err, "failed to update status")
	}
}

func (r *Reconciler) updateStatusError(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
	err error,
) (ctrl.Result, error) {
	updateErr := controller.UpdateStatusWithConflictRetry(ctx, r.Client, cert, func() {
		cert.Status.State = networkingv1alpha2.OriginCACertificateStateError
		cert.Status.Message = cf.SanitizeErrorMessage(err)
		meta.SetStatusCondition(&cert.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cert.Generation,
			Reason:             "Error",
			Message:            cf.SanitizeErrorMessage(err),
			LastTransitionTime: metav1.Now(),
		})
		cert.Status.ObservedGeneration = cert.Generation
	})

	if updateErr != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", updateErr)
	}

	return common.RequeueShort(), nil
}

func (r *Reconciler) updateStatusReady(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
) (ctrl.Result, error) {
	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, cert, func() {
		cert.Status.State = networkingv1alpha2.OriginCACertificateStateReady
		cert.Status.Message = "Certificate is ready"
		meta.SetStatusCondition(&cert.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cert.Generation,
			Reason:             "CertificateReady",
			Message:            "Certificate is ready",
			LastTransitionTime: metav1.Now(),
		})
		cert.Status.ObservedGeneration = cert.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	requeueAfter := r.calculateRequeueTime(cert)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *Reconciler) updateStatusWithCertificate(
	ctx context.Context,
	cert *networkingv1alpha2.OriginCACertificate,
	result *cf.OriginCACertificateResult,
	privateKey []byte,
) (ctrl.Result, error) {
	// Sync to Secret if configured
	if cert.Spec.SecretSync != nil && cert.Spec.SecretSync.Enabled && len(privateKey) > 0 {
		// Temporarily store certificate for sync
		cert.Status.Certificate = result.Certificate
		if err := r.syncSecretWithKey(ctx, cert, privateKey); err != nil {
			r.Recorder.Event(cert, corev1.EventTypeWarning, "SecretSyncFailed", err.Error())
		}
	}

	// Calculate renewal time
	var renewalTime *metav1.Time
	if cert.Spec.Renewal != nil && cert.Spec.Renewal.Enabled {
		renewBeforeDays := cert.Spec.Renewal.RenewBeforeDays
		if renewBeforeDays == 0 {
			renewBeforeDays = 30
		}
		renewalTimeVal := result.ExpiresOn.AddDate(0, 0, -renewBeforeDays)
		renewalTime = &metav1.Time{Time: renewalTimeVal}
	}

	now := metav1.Now()
	expiresAt := metav1.NewTime(result.ExpiresOn)

	err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, cert, func() {
		cert.Status.CertificateID = result.ID
		cert.Status.Certificate = result.Certificate
		cert.Status.IssuedAt = &now
		cert.Status.ExpiresAt = &expiresAt
		cert.Status.RenewalTime = renewalTime
		cert.Status.State = networkingv1alpha2.OriginCACertificateStateReady
		cert.Status.Message = "Certificate is ready"
		meta.SetStatusCondition(&cert.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cert.Generation,
			Reason:             "CertificateReady",
			Message:            "Certificate is ready",
			LastTransitionTime: metav1.Now(),
		})
		cert.Status.ObservedGeneration = cert.Generation
	})

	if err != nil {
		return common.NoRequeue(), fmt.Errorf("failed to update status: %w", err)
	}

	requeueAfter := r.calculateRequeueTime(cert)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// findCertificatesForCredentials returns OriginCACertificates that reference the given credentials.
func (r *Reconciler) findCertificatesForCredentials(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	creds, ok := obj.(*networkingv1alpha2.CloudflareCredentials)
	if !ok {
		return nil
	}
	logger := log.FromContext(ctx)

	certList := &networkingv1alpha2.OriginCACertificateList{}
	if err := r.List(ctx, certList); err != nil {
		logger.Error(err, "Failed to list OriginCACertificates for credentials watch")
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
	r.Recorder = mgr.GetEventRecorderFor("origincacertificate-controller")

	// Initialize APIClientFactory
	r.APIFactory = common.NewAPIClientFactory(mgr.GetClient(), ctrl.Log.WithName("origincacertificate"))

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
