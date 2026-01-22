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
	var warnings admission.Warnings

	// Validate version management
	errs, warns := v.validateVersionManagement(project)
	allErrs = append(allErrs, errs...)
	warnings = append(warnings, warns...)

	// Legacy validation (if using old fields)
	if project.Spec.VersionManagement == nil {
		if err := v.validateProductionTarget(project); err != nil {
			allErrs = append(allErrs, err)
		}

		if err := v.validateVersionUniqueness(project); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: GroupVersion.Group, Kind: "PagesProject"},
			project.Name, allErrs)
	}
	return warnings, nil
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

// validateVersionManagement validates the versionManagement configuration.
func (v *PagesProjectValidator) validateVersionManagement(project *PagesProject) (field.ErrorList, admission.Warnings) {
	var errs field.ErrorList
	var warnings admission.Warnings
	vm := project.Spec.VersionManagement

	// Check for mutual exclusivity with legacy fields
	if vm != nil && (len(project.Spec.Versions) > 0 || project.Spec.ProductionTarget != "") {
		warnings = append(warnings,
			"Both versionManagement and deprecated fields (versions/productionTarget) are specified. "+
				"versionManagement will take precedence. Consider removing the deprecated fields.")
	}

	if vm == nil {
		return errs, warnings
	}

	path := field.NewPath("spec", "versionManagement")

	// Validate type and corresponding configuration
	switch vm.Type {
	case TargetVersionMode:
		if vm.TargetVersion == nil {
			errs = append(errs, field.Required(path.Child("targetVersion"),
				"targetVersion is required when type is targetVersion"))
		} else {
			errs = append(errs, v.validateSourceTemplate(
				path.Child("targetVersion", "sourceTemplate"),
				&vm.TargetVersion.SourceTemplate)...)
		}
		if vm.DeclarativeVersions != nil {
			errs = append(errs, field.Forbidden(path.Child("declarativeVersions"),
				"declarativeVersions must not be set when type is targetVersion"))
		}
		if vm.FullVersions != nil {
			errs = append(errs, field.Forbidden(path.Child("fullVersions"),
				"fullVersions must not be set when type is targetVersion"))
		}

	case DeclarativeVersionsMode:
		if vm.DeclarativeVersions == nil {
			errs = append(errs, field.Required(path.Child("declarativeVersions"),
				"declarativeVersions is required when type is declarativeVersions"))
		} else {
			errs = append(errs, v.validateDeclarativeVersions(
				path.Child("declarativeVersions"), vm.DeclarativeVersions)...)
			errs = append(errs, v.validateSourceTemplate(
				path.Child("declarativeVersions", "sourceTemplate"),
				&vm.DeclarativeVersions.SourceTemplate)...)
		}
		if vm.TargetVersion != nil {
			errs = append(errs, field.Forbidden(path.Child("targetVersion"),
				"targetVersion must not be set when type is declarativeVersions"))
		}
		if vm.FullVersions != nil {
			errs = append(errs, field.Forbidden(path.Child("fullVersions"),
				"fullVersions must not be set when type is declarativeVersions"))
		}

	case FullVersionsMode:
		if vm.FullVersions == nil {
			errs = append(errs, field.Required(path.Child("fullVersions"),
				"fullVersions is required when type is fullVersions"))
		} else {
			errs = append(errs, v.validateFullVersions(
				path.Child("fullVersions"), vm.FullVersions)...)
		}
		if vm.TargetVersion != nil {
			errs = append(errs, field.Forbidden(path.Child("targetVersion"),
				"targetVersion must not be set when type is fullVersions"))
		}
		if vm.DeclarativeVersions != nil {
			errs = append(errs, field.Forbidden(path.Child("declarativeVersions"),
				"declarativeVersions must not be set when type is fullVersions"))
		}

	default:
		errs = append(errs, field.Invalid(path.Child("type"), vm.Type,
			"must be one of: targetVersion, declarativeVersions, fullVersions"))
	}

	return errs, warnings
}

// validateSourceTemplate validates a source template configuration.
func (v *PagesProjectValidator) validateSourceTemplate(path *field.Path, st *SourceTemplate) field.ErrorList {
	var errs field.ErrorList

	switch st.Type {
	case S3SourceTemplateType:
		if st.S3 == nil {
			errs = append(errs, field.Required(path.Child("s3"),
				"s3 is required when type is s3"))
		}
		if st.HTTP != nil {
			errs = append(errs, field.Forbidden(path.Child("http"),
				"http must not be set when type is s3"))
		}
		if st.OCI != nil {
			errs = append(errs, field.Forbidden(path.Child("oci"),
				"oci must not be set when type is s3"))
		}

	case HTTPSourceTemplateType:
		if st.HTTP == nil {
			errs = append(errs, field.Required(path.Child("http"),
				"http is required when type is http"))
		}
		if st.S3 != nil {
			errs = append(errs, field.Forbidden(path.Child("s3"),
				"s3 must not be set when type is http"))
		}
		if st.OCI != nil {
			errs = append(errs, field.Forbidden(path.Child("oci"),
				"oci must not be set when type is http"))
		}

	case OCISourceTemplateType:
		if st.OCI == nil {
			errs = append(errs, field.Required(path.Child("oci"),
				"oci is required when type is oci"))
		}
		if st.S3 != nil {
			errs = append(errs, field.Forbidden(path.Child("s3"),
				"s3 must not be set when type is oci"))
		}
		if st.HTTP != nil {
			errs = append(errs, field.Forbidden(path.Child("http"),
				"http must not be set when type is oci"))
		}

	default:
		errs = append(errs, field.Invalid(path.Child("type"), st.Type,
			"must be one of: s3, http, oci"))
	}

	return errs
}

// validateDeclarativeVersions validates declarative versions configuration.
func (v *PagesProjectValidator) validateDeclarativeVersions(path *field.Path, dv *DeclarativeVersionsSpec) field.ErrorList {
	var errs field.ErrorList

	// Check version uniqueness
	seen := make(map[string]bool)
	for i, ver := range dv.Versions {
		if seen[ver] {
			errs = append(errs, field.Duplicate(
				path.Child("versions").Index(i), ver))
		}
		seen[ver] = true
	}

	// Validate production target if specified
	if dv.ProductionTarget != "" && dv.ProductionTarget != "latest" {
		found := false
		for _, ver := range dv.Versions {
			if ver == dv.ProductionTarget {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, field.Invalid(
				path.Child("productionTarget"), dv.ProductionTarget,
				fmt.Sprintf("version %q not found in versions list", dv.ProductionTarget)))
		}
	}

	return errs
}

// validateFullVersions validates full versions configuration.
func (v *PagesProjectValidator) validateFullVersions(path *field.Path, fv *FullVersionsSpec) field.ErrorList {
	var errs field.ErrorList

	// Check version uniqueness
	seen := make(map[string]bool)
	for i, ver := range fv.Versions {
		if seen[ver.Name] {
			errs = append(errs, field.Duplicate(
				path.Child("versions").Index(i).Child("name"), ver.Name))
		}
		seen[ver.Name] = true
	}

	// Validate production target if specified
	if fv.ProductionTarget != "" && fv.ProductionTarget != "latest" {
		found := false
		for _, ver := range fv.Versions {
			if ver.Name == fv.ProductionTarget {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, field.Invalid(
				path.Child("productionTarget"), fv.ProductionTarget,
				fmt.Sprintf("version %q not found in versions list", fv.ProductionTarget)))
		}
	}

	return errs
}
