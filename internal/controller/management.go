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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ManagementInfo represents the K8s resource managing a Cloudflare resource
type ManagementInfo struct {
	Kind      string
	Namespace string // Empty for cluster-scoped resources
	Name      string
}

// String returns the management marker string for embedding in comments
func (m ManagementInfo) String() string {
	if m.Namespace != "" {
		return fmt.Sprintf("%s%s/%s/%s%s", ManagementMarkerPrefix, m.Kind, m.Namespace, m.Name, ManagementMarkerSuffix)
	}
	return fmt.Sprintf("%s%s/%s%s", ManagementMarkerPrefix, m.Kind, m.Name, ManagementMarkerSuffix)
}

// Equals returns true if two ManagementInfo are equal
func (m ManagementInfo) Equals(other ManagementInfo) bool {
	return m.Kind == other.Kind && m.Namespace == other.Namespace && m.Name == other.Name
}

// NewManagementInfo creates a ManagementInfo from a K8s object
func NewManagementInfo(obj metav1.Object, kind string) ManagementInfo {
	return ManagementInfo{
		Kind:      kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// ParseManagementMarker extracts ManagementInfo from a comment string.
// Returns nil if no management marker is found.
func ParseManagementMarker(comment string) *ManagementInfo {
	startIdx := strings.Index(comment, ManagementMarkerPrefix)
	if startIdx == -1 {
		return nil
	}

	endIdx := strings.Index(comment[startIdx:], ManagementMarkerSuffix)
	if endIdx == -1 {
		return nil
	}

	// Extract the content between prefix and suffix
	markerContent := comment[startIdx+len(ManagementMarkerPrefix) : startIdx+endIdx]

	parts := strings.Split(markerContent, "/")
	if len(parts) == 2 {
		// Cluster-scoped: kind/name
		return &ManagementInfo{
			Kind: parts[0],
			Name: parts[1],
		}
	} else if len(parts) == 3 {
		// Namespace-scoped: kind/namespace/name
		return &ManagementInfo{
			Kind:      parts[0],
			Namespace: parts[1],
			Name:      parts[2],
		}
	}

	return nil
}

// BuildManagedComment creates a comment with management marker prepended.
// If userComment is empty, only the marker is returned.
func BuildManagedComment(info ManagementInfo, userComment string) string {
	marker := info.String()
	if userComment == "" {
		return marker
	}
	return marker + " " + userComment
}

// ExtractUserComment removes the management marker from a comment and returns the user portion.
func ExtractUserComment(comment string) string {
	startIdx := strings.Index(comment, ManagementMarkerPrefix)
	if startIdx == -1 {
		return comment
	}

	endIdx := strings.Index(comment[startIdx:], ManagementMarkerSuffix)
	if endIdx == -1 {
		return comment
	}

	// Remove the marker and any trailing space
	afterMarker := strings.TrimPrefix(comment[startIdx+endIdx+1:], " ")
	beforeMarker := strings.TrimSuffix(comment[:startIdx], " ")

	if beforeMarker != "" && afterMarker != "" {
		return beforeMarker + " " + afterMarker
	}
	return beforeMarker + afterMarker
}

// CanManageResource checks if the given K8s resource can manage a Cloudflare resource.
// Returns true if:
// - The Cloudflare resource has no management marker (first claim)
// - The management marker matches the K8s resource (same owner)
// Returns false if the Cloudflare resource is managed by a different K8s resource.
func CanManageResource(comment string, info ManagementInfo) bool {
	existing := ParseManagementMarker(comment)
	if existing == nil {
		// No existing management, can claim
		return true
	}
	// Can manage only if same owner
	return existing.Equals(info)
}

// GetConflictingManager returns the ManagementInfo of the resource that conflicts with the given info.
// Returns nil if there's no conflict.
func GetConflictingManager(comment string, info ManagementInfo) *ManagementInfo {
	existing := ParseManagementMarker(comment)
	if existing == nil || existing.Equals(info) {
		return nil
	}
	return existing
}
