// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"github.com/cloudflare/cloudflare-go"
)

func TestFindAccessTag(t *testing.T) {
	tags := []cloudflare.AccessTag{
		{Name: "monitoring", AppCount: 3},
		{Name: "backups", AppCount: 0},
	}

	t.Run("found returns converted result", func(t *testing.T) {
		got := findAccessTag(tags, "monitoring")
		if got == nil {
			t.Fatal("expected a result, got nil")
		}
		if got.Name != "monitoring" || got.AppCount != 3 {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("found with zero app count", func(t *testing.T) {
		got := findAccessTag(tags, "backups")
		if got == nil || got.AppCount != 0 {
			t.Fatalf("expected backups with AppCount 0, got %+v", got)
		}
	})

	t.Run("not found returns nil", func(t *testing.T) {
		if got := findAccessTag(tags, "nope"); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("empty list returns nil", func(t *testing.T) {
		if got := findAccessTag(nil, "monitoring"); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
}
