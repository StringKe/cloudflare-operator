// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"reflect"
	"testing"

	"github.com/cloudflare/cloudflare-go"
)

func TestFindAccessTag(t *testing.T) {
	tags := []cloudflare.AccessTag{
		{Name: "monitoring", AppCount: 3},
		{Name: "backups", AppCount: 0},
	}

	tests := []struct {
		name  string
		tags  []cloudflare.AccessTag
		query string
		want  *AccessTagResult
	}{
		{name: "found returns converted result", tags: tags, query: "monitoring", want: &AccessTagResult{Name: "monitoring", AppCount: 3}},
		{name: "found with zero app count", tags: tags, query: "backups", want: &AccessTagResult{Name: "backups", AppCount: 0}},
		{name: "not found returns nil", tags: tags, query: "nope", want: nil},
		{name: "empty list returns nil", tags: nil, query: "monitoring", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAccessTag(tt.tags, tt.query)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("findAccessTag(%q) = %+v, want %+v", tt.query, got, tt.want)
			}
		})
	}
}
