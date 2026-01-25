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

// getEffectivePolicy returns the effective version management policy.
func getEffectivePolicy(vm *VersionManagement) VersionPolicy {
	if vm == nil {
		return VersionPolicyNone
	}
	if vm.Policy != "" {
		return vm.Policy
	}
	return VersionPolicyNone
}

// validateVersionManagement validates the versionManagement configuration.
//
//nolint:revive // cognitive complexity acceptable for validation
func (v *PagesProjectValidator) validateVersionManagement(project *PagesProject) (field.ErrorList, admission.Warnings) {
	var errs field.ErrorList
	var warnings admission.Warnings
	vm := project.Spec.VersionManagement

	if vm == nil {
		return errs, warnings
	}

	path := field.NewPath("spec", "versionManagement")
	policy := getEffectivePolicy(vm)

	// Validate policy and corresponding configuration
	switch policy {
	case VersionPolicyNone, "":
		// No validation needed for none policy

	case VersionPolicyTargetVersion:
		if vm.TargetVersion == nil {
			errs = append(errs, field.Required(path.Child("targetVersion"),
				"targetVersion is required when policy is targetVersion"))
		} else {
			errs = append(errs, v.validateSourceTemplate(
				path.Child("targetVersion", "sourceTemplate"),
				&vm.TargetVersion.SourceTemplate)...)
		}

	case VersionPolicyDeclarativeVersions:
		if vm.DeclarativeVersions == nil {
			errs = append(errs, field.Required(path.Child("declarativeVersions"),
				"declarativeVersions is required when policy is declarativeVersions"))
		} else {
			errs = append(errs, v.validateDeclarativeVersions(
				path.Child("declarativeVersions"), vm.DeclarativeVersions)...)
			errs = append(errs, v.validateSourceTemplate(
				path.Child("declarativeVersions", "sourceTemplate"),
				&vm.DeclarativeVersions.SourceTemplate)...)
		}

	case VersionPolicyFullVersions:
		if vm.FullVersions == nil {
			errs = append(errs, field.Required(path.Child("fullVersions"),
				"fullVersions is required when policy is fullVersions"))
		} else {
			errs = append(errs, v.validateFullVersions(
				path.Child("fullVersions"), vm.FullVersions)...)
		}

	case VersionPolicyGitOps:
		if vm.GitOps == nil {
			errs = append(errs, field.Required(path.Child("gitops"),
				"gitops is required when policy is gitops"))
		} else {
			errs = append(errs, v.validateGitOps(path.Child("gitops"), vm.GitOps)...)
		}

	case VersionPolicyLatestPreview:
		// latestPreview has no required fields, just validate if present
		if vm.LatestPreview != nil && vm.LatestPreview.SourceTemplate != nil {
			errs = append(errs, v.validateSourceTemplate(
				path.Child("latestPreview", "sourceTemplate"),
				vm.LatestPreview.SourceTemplate)...)
		}

	case VersionPolicyAutoPromote:
		// autoPromote has no required fields, just validate if present
		if vm.AutoPromote != nil && vm.AutoPromote.SourceTemplate != nil {
			errs = append(errs, v.validateSourceTemplate(
				path.Child("autoPromote", "sourceTemplate"),
				vm.AutoPromote.SourceTemplate)...)
		}

	case VersionPolicyExternal:
		// external has no required fields

	default:
		errs = append(errs, field.Invalid(path.Child("policy"), policy,
			"must be one of: none, targetVersion, declarativeVersions, fullVersions, gitops, latestPreview, autoPromote, external"))
	}

	return errs, warnings
}

// validateGitOps validates GitOps configuration.
func (v *PagesProjectValidator) validateGitOps(path *field.Path, gitops *GitOpsVersionConfig) field.ErrorList {
	var errs field.ErrorList

	// At least one version should be specified
	if gitops.PreviewVersion == "" && gitops.ProductionVersion == "" {
		warnings := "GitOps policy has no previewVersion or productionVersion specified. " +
			"Consider specifying at least one version."
		_ = warnings // warnings handled differently
	}

	// Validate source template if present
	if gitops.SourceTemplate != nil {
		errs = append(errs, v.validateSourceTemplate(path.Child("sourceTemplate"), gitops.SourceTemplate)...)
	}

	return errs
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
