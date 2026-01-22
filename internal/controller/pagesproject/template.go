// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// TemplateData contains data for template execution.
type TemplateData struct {
	Version string
}

// executeTemplate executes a template string with the given data.
func executeTemplate(tmplStr string, data TemplateData) (string, error) {
	tmpl, err := template.New("source").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// resolveFromS3Template creates a ProjectVersion from an S3 source template.
func resolveFromS3Template(versionName string, s3Tmpl *networkingv1alpha2.S3SourceTemplate) (networkingv1alpha2.ProjectVersion, error) {
	data := TemplateData{Version: versionName}

	key, err := executeTemplate(s3Tmpl.KeyTemplate, data)
	if err != nil {
		return networkingv1alpha2.ProjectVersion{}, fmt.Errorf("execute key template: %w", err)
	}

	archiveType := s3Tmpl.ArchiveType
	if archiveType == "" {
		archiveType = "tar.gz"
	}

	s3Source := &networkingv1alpha2.S3Source{
		Bucket:       s3Tmpl.Bucket,
		Key:          key,
		Region:       s3Tmpl.Region,
		Endpoint:     s3Tmpl.Endpoint,
		UsePathStyle: s3Tmpl.UsePathStyle,
	}

	// Set credentials secret ref if specified
	if s3Tmpl.CredentialsSecretRef != "" {
		s3Source.CredentialsSecretRef = &corev1.LocalObjectReference{
			Name: s3Tmpl.CredentialsSecretRef,
		}
	}

	return networkingv1alpha2.ProjectVersion{
		Name: versionName,
		Source: &networkingv1alpha2.PagesDirectUploadSourceSpec{
			Source: &networkingv1alpha2.DirectUploadSource{
				S3: s3Source,
			},
			Archive: &networkingv1alpha2.ArchiveConfig{
				Type: archiveType,
			},
		},
	}, nil
}

// resolveFromHTTPTemplate creates a ProjectVersion from an HTTP source template.
func resolveFromHTTPTemplate(versionName string, httpTmpl *networkingv1alpha2.HTTPSourceTemplate) (networkingv1alpha2.ProjectVersion, error) {
	data := TemplateData{Version: versionName}

	url, err := executeTemplate(httpTmpl.URLTemplate, data)
	if err != nil {
		return networkingv1alpha2.ProjectVersion{}, fmt.Errorf("execute URL template: %w", err)
	}

	archiveType := httpTmpl.ArchiveType
	if archiveType == "" {
		archiveType = "tar.gz"
	}

	httpSource := &networkingv1alpha2.HTTPSource{
		URL: url,
	}

	// Set headers secret ref if specified
	if httpTmpl.HeadersSecretRef != "" {
		httpSource.HeadersSecretRef = &corev1.LocalObjectReference{
			Name: httpTmpl.HeadersSecretRef,
		}
	}

	return networkingv1alpha2.ProjectVersion{
		Name: versionName,
		Source: &networkingv1alpha2.PagesDirectUploadSourceSpec{
			Source: &networkingv1alpha2.DirectUploadSource{
				HTTP: httpSource,
			},
			Archive: &networkingv1alpha2.ArchiveConfig{
				Type: archiveType,
			},
		},
	}, nil
}

// resolveFromOCITemplate creates a ProjectVersion from an OCI source template.
func resolveFromOCITemplate(versionName string, ociTmpl *networkingv1alpha2.OCISourceTemplate) (networkingv1alpha2.ProjectVersion, error) {
	data := TemplateData{Version: versionName}

	tag, err := executeTemplate(ociTmpl.TagTemplate, data)
	if err != nil {
		return networkingv1alpha2.ProjectVersion{}, fmt.Errorf("execute tag template: %w", err)
	}

	ociSource := &networkingv1alpha2.OCISource{
		Image: fmt.Sprintf("%s:%s", ociTmpl.Repository, tag),
	}

	// Set credentials secret ref if specified
	if ociTmpl.CredentialsSecretRef != "" {
		ociSource.CredentialsSecretRef = &corev1.LocalObjectReference{
			Name: ociTmpl.CredentialsSecretRef,
		}
	}

	return networkingv1alpha2.ProjectVersion{
		Name: versionName,
		Source: &networkingv1alpha2.PagesDirectUploadSourceSpec{
			Source: &networkingv1alpha2.DirectUploadSource{
				OCI: ociSource,
			},
		},
	}, nil
}

// resolveFromTemplate creates a ProjectVersion from a source template.
func resolveFromTemplate(versionName string, tmpl *networkingv1alpha2.SourceTemplate) (networkingv1alpha2.ProjectVersion, error) {
	switch tmpl.Type {
	case networkingv1alpha2.S3SourceTemplateType:
		if tmpl.S3 == nil {
			return networkingv1alpha2.ProjectVersion{}, errors.New("s3 template is nil")
		}
		return resolveFromS3Template(versionName, tmpl.S3)

	case networkingv1alpha2.HTTPSourceTemplateType:
		if tmpl.HTTP == nil {
			return networkingv1alpha2.ProjectVersion{}, errors.New("http template is nil")
		}
		return resolveFromHTTPTemplate(versionName, tmpl.HTTP)

	case networkingv1alpha2.OCISourceTemplateType:
		if tmpl.OCI == nil {
			return networkingv1alpha2.ProjectVersion{}, errors.New("oci template is nil")
		}
		return resolveFromOCITemplate(versionName, tmpl.OCI)

	default:
		return networkingv1alpha2.ProjectVersion{}, fmt.Errorf("unknown source template type: %s", tmpl.Type)
	}
}
