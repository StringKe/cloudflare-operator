// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager sets up the webhook with the Manager.
func (r *PagesProject) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&PagesProjectValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-networking-cloudflare-operator-io-v1alpha2-pagesproject,mutating=false,failurePolicy=fail,sideEffects=None,groups=networking.cloudflare-operator.io,resources=pagesprojects,verbs=create;update,versions=v1alpha2,name=vpagesproject.kb.io,admissionReviewVersions=v1

// PagesProjectValidator implements webhook validation for PagesProject.
type PagesProjectValidator struct{}

var _ webhook.CustomValidator = &PagesProjectValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *PagesProjectValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	project, ok := obj.(*PagesProject)
	if !ok {
		return nil, fmt.Errorf("expected PagesProject but got %T", obj)
	}

	var allErrs field.ErrorList

	if err := v.validateProductionTarget(project); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := v.validateVersionUniqueness(project); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: GroupVersion.Group, Kind: "PagesProject"},
			project.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *PagesProjectValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// Same validation rules as create
	return v.ValidateCreate(ctx, newObj)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *PagesProjectValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for deletion
	return nil, nil
}

// validateProductionTarget validates that the production target references an existing version.
func (v *PagesProjectValidator) validateProductionTarget(project *PagesProject) *field.Error {
	if project.Spec.ProductionTarget == "" || project.Spec.ProductionTarget == "latest" {
		return nil
	}

	// Check if the specified version exists
	for _, ver := range project.Spec.Versions {
		if ver.Name == project.Spec.ProductionTarget {
			return nil
		}
	}

	return field.Invalid(
		field.NewPath("spec", "productionTarget"),
		project.Spec.ProductionTarget,
		fmt.Sprintf("version %q not found in spec.versions", project.Spec.ProductionTarget))
}

// validateVersionUniqueness ensures all version names are unique.
func (v *PagesProjectValidator) validateVersionUniqueness(project *PagesProject) *field.Error {
	seen := make(map[string]bool)
	for i, ver := range project.Spec.Versions {
		if seen[ver.Name] {
			return field.Duplicate(
				field.NewPath("spec", "versions").Index(i).Child("name"), ver.Name)
		}
		seen[ver.Name] = true
	}
	return nil
}
