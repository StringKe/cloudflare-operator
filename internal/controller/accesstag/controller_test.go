// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accesstag

import "testing"

func TestShouldDeleteTag(t *testing.T) {
	tests := []struct {
		name     string
		appCount int
		want     bool
	}{
		{name: "unused tag is deletable", appCount: 0, want: true},
		{name: "tag used by one app is retained", appCount: 1, want: false},
		{name: "tag used by many apps is retained", appCount: 25, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDeleteTag(tt.appCount); got != tt.want {
				t.Fatalf("shouldDeleteTag(%d) = %v, want %v", tt.appCount, got, tt.want)
			}
		})
	}
}
