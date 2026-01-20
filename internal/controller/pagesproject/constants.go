// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package pagesproject

const (
	// FinalizerName is the finalizer for PagesProject resources.
	FinalizerName = "pagesproject.networking.cloudflare-operator.io/finalizer"

	// Managed deployment labels

	// ManagedByLabel identifies resources managed by PagesProject controller.
	ManagedByLabel = "networking.cloudflare-operator.io/managed-by"

	// ManagedByNameLabel stores the PagesProject name that manages this deployment.
	ManagedByNameLabel = "networking.cloudflare-operator.io/managed-by-name"

	// ManagedByUIDLabel stores the PagesProject UID to prevent conflicts after deletion.
	ManagedByUIDLabel = "networking.cloudflare-operator.io/managed-by-uid"

	// VersionLabel stores the version name from ProjectVersion.
	VersionLabel = "networking.cloudflare-operator.io/version"

	// Managed deployment annotations

	// ManagedAnnotation marks a resource as managed by PagesProject.
	ManagedAnnotation = "networking.cloudflare-operator.io/managed"

	// Label values

	// ManagedByValue is the value for ManagedByLabel.
	ManagedByValue = "pagesproject"
)
