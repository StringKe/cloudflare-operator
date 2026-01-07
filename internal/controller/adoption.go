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

package controller

import (
	"fmt"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// AdoptionAnnotation is the annotation key for marking Cloudflare resources as managed
const AdoptionAnnotation = "cloudflare-operator.io/managed-by"

// AdoptionChecker provides utilities for checking if Cloudflare resources
// are already managed by another Kubernetes object
type AdoptionChecker struct {
	// ManagedByValue is the value to use for the managed-by annotation
	// typically in the format "namespace/name"
	ManagedByValue string
}

// NewAdoptionChecker creates a new AdoptionChecker
func NewAdoptionChecker(namespace, name string) *AdoptionChecker {
	value := name
	if namespace != "" {
		value = namespace + "/" + name
	}
	return &AdoptionChecker{
		ManagedByValue: value,
	}
}

// AdoptionResult represents the result of an adoption check
type AdoptionResult struct {
	// Found indicates if the resource was found in Cloudflare
	Found bool
	// CanAdopt indicates if the resource can be adopted by this controller
	CanAdopt bool
	// ExistingID is the ID of the existing resource (if found)
	ExistingID string
	// ManagedBy is the current manager of the resource (if any)
	ManagedBy string
	// Error contains any error that occurred during the check
	Error error
}

// IsAvailable returns true if the resource is available for adoption
// (either not found or can be adopted)
func (r AdoptionResult) IsAvailable() bool {
	return !r.Found || r.CanAdopt
}

// CheckByName checks if a resource with the given name exists and can be adopted
// lookupFn should return (id, managedBy, error) for the resource with the given name
// If the resource is not found, lookupFn should return ("", "", nil)
func (c *AdoptionChecker) CheckByName(name string, lookupFn func(name string) (id string, managedBy string, err error)) AdoptionResult {
	id, managedBy, err := lookupFn(name)
	if err != nil {
		// Check if it's a not found error
		if cf.IsNotFoundError(err) {
			return AdoptionResult{
				Found:    false,
				CanAdopt: true,
			}
		}
		return AdoptionResult{
			Error: err,
		}
	}

	if id == "" {
		// Resource not found
		return AdoptionResult{
			Found:    false,
			CanAdopt: true,
		}
	}

	// Resource found, check if we can adopt it
	if managedBy == "" || managedBy == c.ManagedByValue {
		return AdoptionResult{
			Found:      true,
			CanAdopt:   true,
			ExistingID: id,
			ManagedBy:  managedBy,
		}
	}

	// Resource is managed by another controller
	return AdoptionResult{
		Found:      true,
		CanAdopt:   false,
		ExistingID: id,
		ManagedBy:  managedBy,
	}
}

// ConflictError returns an error for adoption conflict
func (c *AdoptionChecker) ConflictError(resourceType, name, existingManager string) error {
	return fmt.Errorf("%w: %s '%s' is already managed by '%s'",
		cf.ErrResourceConflict, resourceType, name, existingManager)
}

// FormatManagedByValue formats the managed-by annotation value
func FormatManagedByValue(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}
