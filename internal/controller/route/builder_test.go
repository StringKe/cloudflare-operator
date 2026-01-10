// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package route

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// mockBuilder implements Builder for testing
type mockBuilder struct {
	rules []cf.UnvalidatedIngressRule
	err   error
}

func (m *mockBuilder) Build(_ context.Context) ([]cf.UnvalidatedIngressRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rules, nil
}

func TestNewAggregator(t *testing.T) {
	tests := []struct {
		name           string
		fallbackTarget string
		wantFallback   string
	}{
		{
			name:           "default fallback",
			fallbackTarget: "",
			wantFallback:   "http_status:404",
		},
		{
			name:           "custom fallback",
			fallbackTarget: "http_status:503",
			wantFallback:   "http_status:503",
		},
		{
			name:           "service fallback",
			fallbackTarget: "http://fallback:8080",
			wantFallback:   "http://fallback:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg := NewAggregator(tt.fallbackTarget)

			require.NotNil(t, agg)
			assert.Equal(t, tt.wantFallback, agg.FallbackTarget)
			assert.Empty(t, agg.Builders)
		})
	}
}

func TestAggregator_Add(t *testing.T) {
	agg := NewAggregator("")

	builder1 := &mockBuilder{}
	builder2 := &mockBuilder{}

	agg.Add(builder1)
	assert.Len(t, agg.Builders, 1)

	agg.Add(builder2)
	assert.Len(t, agg.Builders, 2)
}

func TestAggregator_AddAll(t *testing.T) {
	agg := NewAggregator("")

	builder1 := &mockBuilder{}
	builder2 := &mockBuilder{}
	builder3 := &mockBuilder{}

	agg.AddAll(builder1, builder2, builder3)
	assert.Len(t, agg.Builders, 3)
}

func TestAggregator_Build_Empty(t *testing.T) {
	agg := NewAggregator("http_status:404")

	rules, err := agg.Build(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "http_status:404", rules[0].Service)
	assert.Empty(t, rules[0].Hostname)
}

func TestAggregator_Build_SingleBuilder(t *testing.T) {
	agg := NewAggregator("")

	builder := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "example.com", Service: "http://app:80"},
		},
	}
	agg.Add(builder)

	rules, err := agg.Build(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "example.com", rules[0].Hostname)
	assert.Equal(t, "http://app:80", rules[0].Service)
	assert.Equal(t, "http_status:404", rules[1].Service) // fallback
}

func TestAggregator_Build_MultipleBuilders(t *testing.T) {
	agg := NewAggregator("")

	builder1 := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "b.example.com", Service: "http://b:80"},
		},
	}
	builder2 := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "a.example.com", Service: "http://a:80"},
		},
	}
	agg.AddAll(builder1, builder2)

	rules, err := agg.Build(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 3)
	// Should be sorted alphabetically by hostname
	assert.Equal(t, "a.example.com", rules[0].Hostname)
	assert.Equal(t, "b.example.com", rules[1].Hostname)
	assert.Equal(t, "http_status:404", rules[2].Service)
}

func TestAggregator_Build_SortByPath(t *testing.T) {
	agg := NewAggregator("")

	builder := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "example.com", Path: "/api", Service: "http://api:80"},
			{Hostname: "example.com", Path: "/", Service: "http://web:80"},
			{Hostname: "example.com", Path: "/admin", Service: "http://admin:80"},
		},
	}
	agg.Add(builder)

	rules, err := agg.Build(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 4)
	// Same hostname, sorted by path
	assert.Equal(t, "/", rules[0].Path)
	assert.Equal(t, "/admin", rules[1].Path)
	assert.Equal(t, "/api", rules[2].Path)
}

func TestAggregator_Build_Error(t *testing.T) {
	agg := NewAggregator("")

	expectedErr := errors.New("build failed")
	builder := &mockBuilder{err: expectedErr}
	agg.Add(builder)

	rules, err := agg.Build(context.Background())

	assert.Nil(t, rules)
	assert.ErrorIs(t, err, expectedErr)
}

func TestAggregator_BuildWithoutFallback_Empty(t *testing.T) {
	agg := NewAggregator("http_status:404")

	rules, err := agg.BuildWithoutFallback(context.Background())

	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestAggregator_BuildWithoutFallback_WithRules(t *testing.T) {
	agg := NewAggregator("")

	builder := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "example.com", Service: "http://app:80"},
		},
	}
	agg.Add(builder)

	rules, err := agg.BuildWithoutFallback(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "example.com", rules[0].Hostname)
	// No fallback rule
}

func TestAggregator_BuildWithoutFallback_Sorted(t *testing.T) {
	agg := NewAggregator("")

	builder := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "z.example.com", Service: "http://z:80"},
			{Hostname: "a.example.com", Service: "http://a:80"},
		},
	}
	agg.Add(builder)

	rules, err := agg.BuildWithoutFallback(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "a.example.com", rules[0].Hostname)
	assert.Equal(t, "z.example.com", rules[1].Hostname)
}

func TestAggregator_BuildWithoutFallback_Error(t *testing.T) {
	agg := NewAggregator("")

	expectedErr := errors.New("build failed")
	builder := &mockBuilder{err: expectedErr}
	agg.Add(builder)

	rules, err := agg.BuildWithoutFallback(context.Background())

	assert.Nil(t, rules)
	assert.ErrorIs(t, err, expectedErr)
}

func TestAggregator_Build_ComplexSorting(t *testing.T) {
	agg := NewAggregator("")

	builder := &mockBuilder{
		rules: []cf.UnvalidatedIngressRule{
			{Hostname: "b.example.com", Path: "/api", Service: "http://b-api:80"},
			{Hostname: "a.example.com", Path: "/web", Service: "http://a-web:80"},
			{Hostname: "b.example.com", Path: "/", Service: "http://b-root:80"},
			{Hostname: "a.example.com", Path: "/api", Service: "http://a-api:80"},
		},
	}
	agg.Add(builder)

	rules, err := agg.Build(context.Background())

	require.NoError(t, err)
	require.Len(t, rules, 5)

	// First sorted by hostname, then by path
	assert.Equal(t, "a.example.com", rules[0].Hostname)
	assert.Equal(t, "/api", rules[0].Path)
	assert.Equal(t, "a.example.com", rules[1].Hostname)
	assert.Equal(t, "/web", rules[1].Path)
	assert.Equal(t, "b.example.com", rules[2].Hostname)
	assert.Equal(t, "/", rules[2].Path)
	assert.Equal(t, "b.example.com", rules[3].Hostname)
	assert.Equal(t, "/api", rules[3].Path)
}
