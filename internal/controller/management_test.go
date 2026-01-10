// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockObject implements metav1.Object for testing
type mockObject struct {
	metav1.ObjectMeta
}

func (*mockObject) GetObjectKind() any  { return nil }
func (*mockObject) DeepCopyObject() any { return nil }

func TestManagementInfoString(t *testing.T) {
	tests := []struct {
		name string
		info ManagementInfo
		want string
	}{
		{
			name: "namespace scoped",
			info: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
			want: ManagementMarkerPrefix + "TunnelBinding/default/my-binding" + ManagementMarkerSuffix,
		},
		{
			name: "cluster scoped",
			info: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			want: ManagementMarkerPrefix + "ClusterTunnel/my-tunnel" + ManagementMarkerSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManagementInfoEquals(t *testing.T) {
	tests := []struct {
		name  string
		info1 ManagementInfo
		info2 ManagementInfo
		want  bool
	}{
		{
			name: "equal namespace scoped",
			info1: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
			info2: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
			want: true,
		},
		{
			name: "equal cluster scoped",
			info1: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			info2: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			want: true,
		},
		{
			name: "different kind",
			info1: ManagementInfo{
				Kind: "TunnelBinding",
				Name: "my-binding",
			},
			info2: ManagementInfo{
				Kind: "Tunnel",
				Name: "my-binding",
			},
			want: false,
		},
		{
			name: "different namespace",
			info1: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
			info2: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "other",
				Name:      "my-binding",
			},
			want: false,
		},
		{
			name: "different name",
			info1: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "tunnel-a",
			},
			info2: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "tunnel-b",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info1.Equals(tt.info2)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewManagementInfo(t *testing.T) {
	tests := []struct {
		name     string
		objName  string
		objNs    string
		kind     string
		wantInfo ManagementInfo
	}{
		{
			name:    "namespace scoped",
			objName: "my-binding",
			objNs:   "default",
			kind:    "TunnelBinding",
			wantInfo: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
		},
		{
			name:    "cluster scoped",
			objName: "my-tunnel",
			objNs:   "",
			kind:    "ClusterTunnel",
			wantInfo: ManagementInfo{
				Kind:      "ClusterTunnel",
				Namespace: "",
				Name:      "my-tunnel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &mockObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.objName,
					Namespace: tt.objNs,
				},
			}

			got := NewManagementInfo(obj, tt.kind)
			assert.Equal(t, tt.wantInfo, got)
		})
	}
}

func TestParseManagementMarker(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    *ManagementInfo
	}{
		{
			name:    "namespace scoped marker",
			comment: ManagementMarkerPrefix + "TunnelBinding/default/my-binding" + ManagementMarkerSuffix,
			want: &ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
		},
		{
			name:    "cluster scoped marker",
			comment: ManagementMarkerPrefix + "ClusterTunnel/my-tunnel" + ManagementMarkerSuffix,
			want: &ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
		},
		{
			name:    "marker with user comment",
			comment: ManagementMarkerPrefix + "TunnelBinding/ns/name" + ManagementMarkerSuffix + " User comment here",
			want: &ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "ns",
				Name:      "name",
			},
		},
		{
			name:    "no marker",
			comment: "Just a regular comment",
			want:    nil,
		},
		{
			name:    "empty comment",
			comment: "",
			want:    nil,
		},
		{
			name:    "prefix only no suffix",
			comment: ManagementMarkerPrefix + "TunnelBinding/ns/name",
			want:    nil,
		},
		{
			name:    "invalid format - single part",
			comment: ManagementMarkerPrefix + "invalid" + ManagementMarkerSuffix,
			want:    nil,
		},
		{
			name:    "invalid format - too many parts",
			comment: ManagementMarkerPrefix + "a/b/c/d/e" + ManagementMarkerSuffix,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseManagementMarker(tt.comment)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want.Kind, got.Kind)
				assert.Equal(t, tt.want.Namespace, got.Namespace)
				assert.Equal(t, tt.want.Name, got.Name)
			}
		})
	}
}

func TestBuildManagedComment(t *testing.T) {
	tests := []struct {
		name        string
		info        ManagementInfo
		userComment string
		want        string
	}{
		{
			name: "with user comment",
			info: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "default",
				Name:      "my-binding",
			},
			userComment: "My DNS record",
			want:        ManagementMarkerPrefix + "TunnelBinding/default/my-binding" + ManagementMarkerSuffix + " My DNS record",
		},
		{
			name: "without user comment",
			info: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			userComment: "",
			want:        ManagementMarkerPrefix + "ClusterTunnel/my-tunnel" + ManagementMarkerSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildManagedComment(tt.info, tt.userComment)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractUserComment(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    string
	}{
		{
			name:    "marker with user comment",
			comment: ManagementMarkerPrefix + "TunnelBinding/ns/name" + ManagementMarkerSuffix + " User comment",
			want:    "User comment",
		},
		{
			name:    "marker only",
			comment: ManagementMarkerPrefix + "TunnelBinding/ns/name" + ManagementMarkerSuffix,
			want:    "",
		},
		{
			name:    "no marker",
			comment: "Just a regular comment",
			want:    "Just a regular comment",
		},
		{
			name:    "empty comment",
			comment: "",
			want:    "",
		},
		{
			name:    "comment before marker",
			comment: "Prefix " + ManagementMarkerPrefix + "TunnelBinding/ns/name" + ManagementMarkerSuffix + " Suffix",
			want:    "Prefix Suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractUserComment(tt.comment)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCanManageResource(t *testing.T) {
	myInfo := ManagementInfo{
		Kind:      "TunnelBinding",
		Namespace: "default",
		Name:      "my-binding",
	}

	otherInfo := ManagementInfo{
		Kind:      "TunnelBinding",
		Namespace: "other",
		Name:      "other-binding",
	}

	tests := []struct {
		name    string
		comment string
		info    ManagementInfo
		want    bool
	}{
		{
			name:    "no existing marker - can claim",
			comment: "Just a comment",
			info:    myInfo,
			want:    true,
		},
		{
			name:    "empty comment - can claim",
			comment: "",
			info:    myInfo,
			want:    true,
		},
		{
			name:    "same owner - can manage",
			comment: myInfo.String(),
			info:    myInfo,
			want:    true,
		},
		{
			name:    "different owner - cannot manage",
			comment: otherInfo.String(),
			info:    myInfo,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanManageResource(tt.comment, tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetConflictingManager(t *testing.T) {
	myInfo := ManagementInfo{
		Kind:      "TunnelBinding",
		Namespace: "default",
		Name:      "my-binding",
	}

	otherInfo := ManagementInfo{
		Kind:      "TunnelBinding",
		Namespace: "other",
		Name:      "other-binding",
	}

	tests := []struct {
		name    string
		comment string
		info    ManagementInfo
		want    *ManagementInfo
	}{
		{
			name:    "no existing marker - no conflict",
			comment: "Just a comment",
			info:    myInfo,
			want:    nil,
		},
		{
			name:    "same owner - no conflict",
			comment: myInfo.String(),
			info:    myInfo,
			want:    nil,
		},
		{
			name:    "different owner - conflict",
			comment: otherInfo.String(),
			info:    myInfo,
			want:    &otherInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetConflictingManager(tt.comment, tt.info)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.want.Kind, got.Kind)
				assert.Equal(t, tt.want.Namespace, got.Namespace)
				assert.Equal(t, tt.want.Name, got.Name)
			}
		})
	}
}

func TestRoundTripManagementMarker(t *testing.T) {
	tests := []struct {
		name string
		info ManagementInfo
	}{
		{
			name: "namespace scoped",
			info: ManagementInfo{
				Kind:      "TunnelBinding",
				Namespace: "my-namespace",
				Name:      "my-resource",
			},
		},
		{
			name: "cluster scoped",
			info: ManagementInfo{
				Kind: "ClusterTunnel",
				Name: "global-tunnel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build comment
			comment := BuildManagedComment(tt.info, "User comment")

			// Parse it back
			parsed := ParseManagementMarker(comment)
			require.NotNil(t, parsed)

			// Verify equality
			assert.True(t, tt.info.Equals(*parsed))

			// Verify user comment extraction
			userComment := ExtractUserComment(comment)
			assert.Equal(t, "User comment", userComment)
		})
	}
}
