// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package transformrule

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, networkingv1alpha2.AddToScheme(scheme))
	return scheme
}

func TestFinalizerName(t *testing.T) {
	assert.Equal(t, "cloudflare.com/transform-rule-finalizer", finalizerName)
	assert.Contains(t, finalizerName, "transform")
}

func TestReconcilerFields(t *testing.T) {
	r := &Reconciler{}

	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.transformRuleService)
}

func TestReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.False(t, result.Requeue)
}

func TestReconciler_Reconcile_WithDeletingRule(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	now := metav1.Now()
	rule := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-rule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone:        "example.com",
			Type:        networkingv1alpha2.TransformRuleTypeURLRewrite,
			Description: "Test transform rule",
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "test-rewrite",
					Expression: "(http.host eq \"example.com\")",
					Enabled:    true,
					URLRewrite: &networkingv1alpha2.URLRewriteConfig{
						Path: &networkingv1alpha2.RewriteValue{
							Static: "/new-path",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(rule).
		WithStatusSubresource(rule).
		Build()

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// This tests the deletion flow path - may succeed or fail based on credentials
	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "test-rule",
			Namespace: "default",
		},
	})

	// The test verifies that reconciliation proceeds (no panic)
	// Behavior depends on whether credentials can be resolved
	_ = result
	_ = err
}

func TestGetPhase(t *testing.T) {
	r := &Reconciler{}

	tests := []struct {
		ruleType      networkingv1alpha2.TransformRuleType
		expectedPhase string
	}{
		{networkingv1alpha2.TransformRuleTypeURLRewrite, "http_request_transform"},
		{networkingv1alpha2.TransformRuleTypeRequestHeader, "http_request_late_transform"},
		{networkingv1alpha2.TransformRuleTypeResponseHeader, "http_response_headers_transform"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ruleType), func(t *testing.T) {
			rule := &networkingv1alpha2.TransformRule{
				Spec: networkingv1alpha2.TransformRuleSpec{
					Type: tt.ruleType,
				},
			}
			phase := r.getPhase(rule)
			assert.Equal(t, tt.expectedPhase, phase)
		})
	}
}

func TestFindRulesForCredentials_TypeCheck(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Pass wrong type - should return nil
	wrongObj := &networkingv1alpha2.TransformRule{}
	result := r.findRulesForCredentials(ctx, wrongObj)
	assert.Nil(t, result)
}

func TestFindRulesForCredentials_MatchingRules(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-creds",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			IsDefault: false,
		},
	}

	rule1 := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-1",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone: "example.com",
			Type: networkingv1alpha2.TransformRuleTypeURLRewrite,
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "my-creds",
			},
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "rewrite-1",
					Expression: "(true)",
					Enabled:    true,
				},
			},
		},
	}

	rule2 := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-2",
			Namespace: "other",
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone: "other.com",
			Type: networkingv1alpha2.TransformRuleTypeRequestHeader,
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "other-creds",
			},
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "header-1",
					Expression: "(true)",
					Enabled:    true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(creds, rule1, rule2).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result := r.findRulesForCredentials(ctx, creds)
	assert.Len(t, result, 1)
	assert.Equal(t, "rule-1", result[0].Name)
	assert.Equal(t, "default", result[0].Namespace)
}

func TestFindRulesForCredentials_DefaultCredentials(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	defaultCreds := &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-creds",
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			IsDefault: true,
		},
	}

	ruleWithoutRef := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-no-ref",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone: "example.com",
			Type: networkingv1alpha2.TransformRuleTypeURLRewrite,
			// No CredentialsRef - should use default
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "rewrite",
					Expression: "(true)",
					Enabled:    true,
				},
			},
		},
	}

	ruleWithRef := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-with-ref",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone: "other.com",
			Type: networkingv1alpha2.TransformRuleTypeRequestHeader,
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "specific-creds",
			},
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "header",
					Expression: "(true)",
					Enabled:    true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultCreds, ruleWithoutRef, ruleWithRef).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result := r.findRulesForCredentials(ctx, defaultCreds)
	assert.Len(t, result, 1)
	assert.Equal(t, "rule-no-ref", result[0].Name)
}

func TestFindRulesForSyncState_TypeCheck(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}

	// Pass wrong type
	wrongObj := &networkingv1alpha2.TransformRule{}
	result := r.findRulesForSyncState(ctx, wrongObj)
	assert.Nil(t, result)
}

func TestFindRulesForSyncState_WrongResourceType(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}

	syncState := &networkingv1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-syncstate",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.CloudflareSyncStateSpec{
			ResourceType: networkingv1alpha2.SyncResourceTunnelConfiguration, // Wrong type
		},
	}

	result := r.findRulesForSyncState(ctx, syncState)
	assert.Nil(t, result)
}

func TestFindRulesForSyncState_MatchingRules(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}

	syncState := &networkingv1alpha2.CloudflareSyncState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-syncstate",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.CloudflareSyncStateSpec{
			ResourceType: networkingv1alpha2.SyncResourceTransformRule,
			Sources: []networkingv1alpha2.ConfigSource{
				{
					Ref: networkingv1alpha2.SourceReference{
						Kind:      "TransformRule",
						Name:      "rule-1",
						Namespace: "default",
					},
				},
				{
					Ref: networkingv1alpha2.SourceReference{
						Kind:      "TransformRule",
						Name:      "rule-2",
						Namespace: "other",
					},
				},
				{
					Ref: networkingv1alpha2.SourceReference{
						Kind:      "OtherKind", // Different kind
						Name:      "rule-3",
						Namespace: "default",
					},
				},
			},
		},
	}

	result := r.findRulesForSyncState(ctx, syncState)
	assert.Len(t, result, 2)
	assert.Contains(t, result, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "rule-1", Namespace: "default"},
	})
	assert.Contains(t, result, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "rule-2", Namespace: "other"},
	})
}

func TestTransformRuleStates(t *testing.T) {
	tests := []struct {
		state    networkingv1alpha2.TransformRuleState
		expected string
	}{
		{networkingv1alpha2.TransformRuleStatePending, "Pending"},
		{networkingv1alpha2.TransformRuleStateSyncing, "Syncing"},
		{networkingv1alpha2.TransformRuleStateReady, "Ready"},
		{networkingv1alpha2.TransformRuleStateError, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.state))
		})
	}
}

func TestTransformRuleTypes(t *testing.T) {
	tests := []struct {
		ruleType networkingv1alpha2.TransformRuleType
		expected string
	}{
		{networkingv1alpha2.TransformRuleTypeURLRewrite, "url_rewrite"},
		{networkingv1alpha2.TransformRuleTypeRequestHeader, "request_header"},
		{networkingv1alpha2.TransformRuleTypeResponseHeader, "response_header"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.ruleType))
		})
	}
}

func TestHeaderOperations(t *testing.T) {
	tests := []struct {
		op       networkingv1alpha2.HeaderOperation
		expected string
	}{
		{networkingv1alpha2.HeaderOperationSet, "set"},
		{networkingv1alpha2.HeaderOperationAdd, "add"},
		{networkingv1alpha2.HeaderOperationRemove, "remove"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.op))
		})
	}
}

func TestURLRewriteConfig(t *testing.T) {
	// Test with static path
	configStatic := networkingv1alpha2.URLRewriteConfig{
		Path: &networkingv1alpha2.RewriteValue{
			Static: "/new-path",
		},
		Query: &networkingv1alpha2.RewriteValue{
			Static: "param=value",
		},
	}

	assert.NotNil(t, configStatic.Path)
	assert.Equal(t, "/new-path", configStatic.Path.Static)
	assert.NotNil(t, configStatic.Query)
	assert.Equal(t, "param=value", configStatic.Query.Static)

	// Test with expression path
	configExpr := networkingv1alpha2.URLRewriteConfig{
		Path: &networkingv1alpha2.RewriteValue{
			Expression: "concat(\"/api/v2\", http.request.uri.path)",
		},
	}

	assert.NotNil(t, configExpr.Path)
	assert.NotEmpty(t, configExpr.Path.Expression)
	assert.Empty(t, configExpr.Path.Static)
	assert.Nil(t, configExpr.Query)
}

func TestRewriteValue(t *testing.T) {
	// Static value
	staticVal := networkingv1alpha2.RewriteValue{
		Static: "/static-path",
	}
	assert.Equal(t, "/static-path", staticVal.Static)
	assert.Empty(t, staticVal.Expression)

	// Expression value
	exprVal := networkingv1alpha2.RewriteValue{
		Expression: "concat(http.request.uri.path, \"/suffix\")",
	}
	assert.Empty(t, exprVal.Static)
	assert.NotEmpty(t, exprVal.Expression)
}

func TestHeaderModification(t *testing.T) {
	// Set operation with static value
	setHeader := networkingv1alpha2.HeaderModification{
		Name:      "X-Custom-Header",
		Operation: networkingv1alpha2.HeaderOperationSet,
		Value:     "custom-value",
	}
	assert.Equal(t, "X-Custom-Header", setHeader.Name)
	assert.Equal(t, networkingv1alpha2.HeaderOperationSet, setHeader.Operation)
	assert.Equal(t, "custom-value", setHeader.Value)
	assert.Empty(t, setHeader.Expression)

	// Add operation with expression
	addHeader := networkingv1alpha2.HeaderModification{
		Name:       "X-Country",
		Operation:  networkingv1alpha2.HeaderOperationAdd,
		Expression: "ip.geoip.country",
	}
	assert.Equal(t, "X-Country", addHeader.Name)
	assert.Equal(t, networkingv1alpha2.HeaderOperationAdd, addHeader.Operation)
	assert.Empty(t, addHeader.Value)
	assert.NotEmpty(t, addHeader.Expression)

	// Remove operation
	removeHeader := networkingv1alpha2.HeaderModification{
		Name:      "Server",
		Operation: networkingv1alpha2.HeaderOperationRemove,
	}
	assert.Equal(t, "Server", removeHeader.Name)
	assert.Equal(t, networkingv1alpha2.HeaderOperationRemove, removeHeader.Operation)
	assert.Empty(t, removeHeader.Value)
	assert.Empty(t, removeHeader.Expression)
}

func TestTransformRuleDefinition_URLRewrite(t *testing.T) {
	def := networkingv1alpha2.TransformRuleDefinition{
		Name:       "rewrite-api-path",
		Expression: "(http.host eq \"api.example.com\")",
		Enabled:    true,
		URLRewrite: &networkingv1alpha2.URLRewriteConfig{
			Path: &networkingv1alpha2.RewriteValue{
				Expression: "concat(\"/v2\", http.request.uri.path)",
			},
		},
	}

	assert.Equal(t, "rewrite-api-path", def.Name)
	assert.Equal(t, "(http.host eq \"api.example.com\")", def.Expression)
	assert.True(t, def.Enabled)
	assert.NotNil(t, def.URLRewrite)
	assert.NotNil(t, def.URLRewrite.Path)
	assert.Nil(t, def.Headers)
}

func TestTransformRuleDefinition_Headers(t *testing.T) {
	def := networkingv1alpha2.TransformRuleDefinition{
		Name:       "add-security-headers",
		Expression: "(true)",
		Enabled:    true,
		Headers: []networkingv1alpha2.HeaderModification{
			{
				Name:      "X-Content-Type-Options",
				Operation: networkingv1alpha2.HeaderOperationSet,
				Value:     "nosniff",
			},
			{
				Name:      "X-Frame-Options",
				Operation: networkingv1alpha2.HeaderOperationSet,
				Value:     "DENY",
			},
			{
				Name:      "Server",
				Operation: networkingv1alpha2.HeaderOperationRemove,
			},
		},
	}

	assert.Equal(t, "add-security-headers", def.Name)
	assert.True(t, def.Enabled)
	assert.Nil(t, def.URLRewrite)
	assert.Len(t, def.Headers, 3)
}

func TestTransformRuleSpec_URLRewrite(t *testing.T) {
	spec := networkingv1alpha2.TransformRuleSpec{
		Zone:        "example.com",
		Type:        networkingv1alpha2.TransformRuleTypeURLRewrite,
		Description: "URL rewrite rules",
		Rules: []networkingv1alpha2.TransformRuleDefinition{
			{
				Name:       "rewrite-1",
				Expression: "(http.request.uri.path starts_with \"/old/\")",
				Enabled:    true,
				URLRewrite: &networkingv1alpha2.URLRewriteConfig{
					Path: &networkingv1alpha2.RewriteValue{
						Expression: "regex_replace(http.request.uri.path, \"^/old/\", \"/new/\")",
					},
				},
			},
		},
	}

	assert.Equal(t, "example.com", spec.Zone)
	assert.Equal(t, networkingv1alpha2.TransformRuleTypeURLRewrite, spec.Type)
	assert.Len(t, spec.Rules, 1)
	assert.NotNil(t, spec.Rules[0].URLRewrite)
}

func TestTransformRuleSpec_RequestHeader(t *testing.T) {
	spec := networkingv1alpha2.TransformRuleSpec{
		Zone:        "example.com",
		Type:        networkingv1alpha2.TransformRuleTypeRequestHeader,
		Description: "Request header modifications",
		Rules: []networkingv1alpha2.TransformRuleDefinition{
			{
				Name:       "add-geo-header",
				Expression: "(true)",
				Enabled:    true,
				Headers: []networkingv1alpha2.HeaderModification{
					{
						Name:       "X-Geo-Country",
						Operation:  networkingv1alpha2.HeaderOperationSet,
						Expression: "ip.geoip.country",
					},
				},
			},
		},
	}

	assert.Equal(t, networkingv1alpha2.TransformRuleTypeRequestHeader, spec.Type)
	assert.Len(t, spec.Rules[0].Headers, 1)
}

func TestTransformRuleSpec_ResponseHeader(t *testing.T) {
	spec := networkingv1alpha2.TransformRuleSpec{
		Zone:        "example.com",
		Type:        networkingv1alpha2.TransformRuleTypeResponseHeader,
		Description: "Response header modifications",
		Rules: []networkingv1alpha2.TransformRuleDefinition{
			{
				Name:       "security-headers",
				Expression: "(true)",
				Enabled:    true,
				Headers: []networkingv1alpha2.HeaderModification{
					{
						Name:      "Strict-Transport-Security",
						Operation: networkingv1alpha2.HeaderOperationSet,
						Value:     "max-age=31536000; includeSubDomains",
					},
					{
						Name:      "X-Content-Type-Options",
						Operation: networkingv1alpha2.HeaderOperationSet,
						Value:     "nosniff",
					},
				},
			},
		},
		CredentialsRef: &networkingv1alpha2.CredentialsReference{
			Name: "my-creds",
		},
	}

	assert.Equal(t, networkingv1alpha2.TransformRuleTypeResponseHeader, spec.Type)
	assert.Len(t, spec.Rules[0].Headers, 2)
	assert.NotNil(t, spec.CredentialsRef)
}

func TestTransformRuleStatus(t *testing.T) {
	status := networkingv1alpha2.TransformRuleStatus{
		State:              networkingv1alpha2.TransformRuleStateReady,
		RulesetID:          "ruleset-123",
		ZoneID:             "zone-456",
		RuleCount:          5,
		Message:            "Transform rules synced successfully",
		ObservedGeneration: 3,
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				Reason:             "Synced",
				Message:            "All transform rules synced",
				ObservedGeneration: 3,
			},
		},
	}

	assert.Equal(t, networkingv1alpha2.TransformRuleStateReady, status.State)
	assert.Equal(t, "ruleset-123", status.RulesetID)
	assert.Equal(t, "zone-456", status.ZoneID)
	assert.Equal(t, 5, status.RuleCount)
	assert.Equal(t, "Transform rules synced successfully", status.Message)
	assert.Equal(t, int64(3), status.ObservedGeneration)
	assert.Len(t, status.Conditions, 1)
}

// Note: TestClearRulesFromCloudflare_NoStatusIDs removed
// clearRulesFromCloudflare method removed following Unified Sync Architecture
// Deletion is now handled by SyncController, not ResourceController

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create a rule without finalizers - not added to fake client
	// This tests the case where a rule being deleted has no finalizer
	rule := &networkingv1alpha2.TransformRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
			// Note: We don't set DeletionTimestamp here because the fake client won't allow it
			// Instead we test the handleDeletion logic directly
		},
		Spec: networkingv1alpha2.TransformRuleSpec{
			Zone: "example.com",
			Type: networkingv1alpha2.TransformRuleTypeURLRewrite,
			Rules: []networkingv1alpha2.TransformRuleDefinition{
				{
					Name:       "test",
					Expression: "(true)",
					Enabled:    true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Test handleDeletion with a rule that has no finalizer
	credRef := networkingv1alpha2.CredentialsReference{Name: "test-creds"}
	result, err := r.handleDeletion(ctx, rule, credRef)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Zero(t, result.RequeueAfter)
}

func TestTransformRuleSpec_WithMultipleRules(t *testing.T) {
	spec := networkingv1alpha2.TransformRuleSpec{
		Zone:        "example.com",
		Type:        networkingv1alpha2.TransformRuleTypeRequestHeader,
		Description: "Multiple request header rules",
		Rules: []networkingv1alpha2.TransformRuleDefinition{
			{
				Name:       "rule-1",
				Expression: "(http.request.uri.path starts_with \"/api/\")",
				Enabled:    true,
				Headers: []networkingv1alpha2.HeaderModification{
					{
						Name:      "X-API-Version",
						Operation: networkingv1alpha2.HeaderOperationSet,
						Value:     "v2",
					},
				},
			},
			{
				Name:       "rule-2",
				Expression: "(http.request.uri.path starts_with \"/internal/\")",
				Enabled:    false,
				Headers: []networkingv1alpha2.HeaderModification{
					{
						Name:      "X-Internal",
						Operation: networkingv1alpha2.HeaderOperationSet,
						Value:     "true",
					},
				},
			},
			{
				Name:       "rule-3",
				Expression: "(true)",
				Enabled:    true,
				Headers: []networkingv1alpha2.HeaderModification{
					{
						Name:       "X-Request-ID",
						Operation:  networkingv1alpha2.HeaderOperationAdd,
						Expression: "cf.ray_id",
					},
				},
			},
		},
	}

	assert.Len(t, spec.Rules, 3)
	assert.True(t, spec.Rules[0].Enabled)
	assert.False(t, spec.Rules[1].Enabled)
	assert.True(t, spec.Rules[2].Enabled)
}

func TestURLRewriteConfig_BothPathAndQuery(t *testing.T) {
	config := networkingv1alpha2.URLRewriteConfig{
		Path: &networkingv1alpha2.RewriteValue{
			Expression: "concat(\"/api/v2\", http.request.uri.path)",
		},
		Query: &networkingv1alpha2.RewriteValue{
			Static: "version=2",
		},
	}

	assert.NotNil(t, config.Path)
	assert.NotEmpty(t, config.Path.Expression)
	assert.NotNil(t, config.Query)
	assert.Equal(t, "version=2", config.Query.Static)
}

func TestHeaderModification_AllOperations(t *testing.T) {
	headers := []networkingv1alpha2.HeaderModification{
		{
			Name:      "Header-To-Set",
			Operation: networkingv1alpha2.HeaderOperationSet,
			Value:     "set-value",
		},
		{
			Name:      "Header-To-Add",
			Operation: networkingv1alpha2.HeaderOperationAdd,
			Value:     "add-value",
		},
		{
			Name:      "Header-To-Remove",
			Operation: networkingv1alpha2.HeaderOperationRemove,
		},
		{
			Name:       "Dynamic-Header",
			Operation:  networkingv1alpha2.HeaderOperationSet,
			Expression: "cf.ray_id",
		},
	}

	assert.Len(t, headers, 4)

	// Verify set operation
	assert.Equal(t, networkingv1alpha2.HeaderOperationSet, headers[0].Operation)
	assert.Equal(t, "set-value", headers[0].Value)

	// Verify add operation
	assert.Equal(t, networkingv1alpha2.HeaderOperationAdd, headers[1].Operation)
	assert.Equal(t, "add-value", headers[1].Value)

	// Verify remove operation
	assert.Equal(t, networkingv1alpha2.HeaderOperationRemove, headers[2].Operation)
	assert.Empty(t, headers[2].Value)

	// Verify dynamic expression
	assert.Equal(t, networkingv1alpha2.HeaderOperationSet, headers[3].Operation)
	assert.NotEmpty(t, headers[3].Expression)
}
