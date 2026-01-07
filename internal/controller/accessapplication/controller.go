/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package accessapplication

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudflare/cloudflare-go"

	networkingv1alpha2 "github.com/adyanth/cloudflare-operator/api/v1alpha2"
	"github.com/adyanth/cloudflare-operator/internal/clients/cf"
	"github.com/adyanth/cloudflare-operator/internal/controller"
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
	secret := &corev1.Secret{}
	secretName := r.app.Spec.Cloudflare.Secret
	namespace := "cloudflare-operator-system"

	if err := r.Get(r.ctx, apitypes.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return err
	}

	apiToken := string(secret.Data[r.app.Spec.Cloudflare.CLOUDFLARE_API_TOKEN])
	apiKey := string(secret.Data[r.app.Spec.Cloudflare.CLOUDFLARE_API_KEY])

	if apiToken == "" && apiKey == "" {
		return fmt.Errorf("no API credentials found")
	}

	var cloudflareClient *cloudflare.API
	var err error
	if apiToken != "" {
		cloudflareClient, err = cloudflare.NewWithAPIToken(apiToken)
	} else {
		cloudflareClient, err = cloudflare.New(apiKey, r.app.Spec.Cloudflare.Email)
	}
	if err != nil {
		return err
	}

	r.cfAPI = &cf.API{
		Log:              r.log,
		AccountName:      r.app.Spec.Cloudflare.AccountName,
		AccountId:        r.app.Spec.Cloudflare.AccountId,
		ValidAccountId:   r.app.Status.AccountID,
		CloudflareClient: cloudflareClient,
	}

	return nil
}

func (r *Reconciler) handleDeletion() (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(r.app, AccessApplicationFinalizer) {
		return ctrl.Result{}, nil
	}

	if r.app.Status.ApplicationID != "" {
		if err := r.cfAPI.DeleteAccessApplication(r.app.Status.ApplicationID); err != nil {
			r.Recorder.Event(r.app, corev1.EventTypeWarning, controller.EventReasonDeleteFailed, err.Error())
			return ctrl.Result{}, err
		}
		r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonDeleted, "Deleted from Cloudflare")
	}

	controllerutil.RemoveFinalizer(r.app, AccessApplicationFinalizer)
	return ctrl.Result{}, r.Update(r.ctx, r.app)
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
		return r.updateStatus(existing)
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
	return r.updateStatus(result)
}

func (r *Reconciler) updateApplication(params cf.AccessApplicationParams) error {
	result, err := r.cfAPI.UpdateAccessApplication(r.app.Status.ApplicationID, params)
	if err != nil {
		r.setCondition(metav1.ConditionFalse, controller.EventReasonUpdateFailed, err.Error())
		return err
	}

	r.Recorder.Event(r.app, corev1.EventTypeNormal, controller.EventReasonUpdated, "Updated AccessApplication")
	return r.updateStatus(result)
}

func (r *Reconciler) updateStatus(result *cf.AccessApplicationResult) error {
	r.app.Status.ApplicationID = result.ID
	r.app.Status.AUD = result.AUD
	r.app.Status.AccountID = r.cfAPI.ValidAccountId
	r.app.Status.Domain = result.Domain
	r.app.Status.State = "active"
	r.app.Status.ObservedGeneration = r.app.Generation

	r.setCondition(metav1.ConditionTrue, controller.EventReasonReconciled, "Reconciled successfully")
	return r.Status().Update(r.ctx, r.app)
}

func (r *Reconciler) setCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: r.app.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	found := false
	for i, c := range r.app.Status.Conditions {
		if c.Type == condition.Type {
			r.app.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		r.app.Status.Conditions = append(r.app.Status.Conditions, condition)
	}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("accessapplication-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha2.AccessApplication{}).
		Complete(r)
}
