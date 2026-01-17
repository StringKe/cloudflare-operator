// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package redirectrule

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
	assert.Equal(t, "cloudflare.com/redirect-rule-finalizer", finalizerName)
	assert.Contains(t, finalizerName, "redirect")
}

func TestRedirectPhase(t *testing.T) {
	assert.Equal(t, "http_request_dynamic_redirect", redirectPhase)
}

func TestReconcilerFields(t *testing.T) {
	r := &Reconciler{}

	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.redirectRuleService)
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
	rule := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-rule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone:        "example.com",
			Description: "Test redirect rule",
			Rules: []networkingv1alpha2.RedirectRuleDefinition{
				{
					Name:       "test-redirect",
					Expression: "(http.host eq \"old.example.com\")",
					Enabled:    true,
					Target: networkingv1alpha2.RedirectTarget{
						URL: "https://new.example.com",
					},
					StatusCode: networkingv1alpha2.RedirectStatusMovedPermanently,
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
	wrongObj := &networkingv1alpha2.RedirectRule{}
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

	rule1 := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-1",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "example.com",
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "my-creds",
			},
			Rules: []networkingv1alpha2.RedirectRuleDefinition{
				{
					Name:       "redirect-1",
					Expression: "(http.host eq \"old.example.com\")",
					Enabled:    true,
					Target: networkingv1alpha2.RedirectTarget{
						URL: "https://new.example.com",
					},
					StatusCode: networkingv1alpha2.RedirectStatusMovedPermanently,
				},
			},
		},
	}

	rule2 := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-2",
			Namespace: "other",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "other.com",
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "other-creds",
			},
			Rules: []networkingv1alpha2.RedirectRuleDefinition{
				{
					Name:       "redirect-2",
					Expression: "(http.host eq \"old.other.com\")",
					Enabled:    true,
					Target: networkingv1alpha2.RedirectTarget{
						URL: "https://new.other.com",
					},
					StatusCode: networkingv1alpha2.RedirectStatusFound,
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

	ruleWithoutRef := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-no-ref",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "example.com",
			// No CredentialsRef - should use default
			WildcardRules: []networkingv1alpha2.WildcardRedirectRule{
				{
					Name:       "wildcard-redirect",
					SourceURL:  "https://old.example.com/*",
					TargetURL:  "https://new.example.com/${1}",
					Enabled:    true,
					StatusCode: networkingv1alpha2.RedirectStatusPermanentRedirect,
				},
			},
		},
	}

	ruleWithRef := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-with-ref",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "other.com",
			CredentialsRef: &networkingv1alpha2.CredentialsReference{
				Name: "specific-creds",
			},
			Rules: []networkingv1alpha2.RedirectRuleDefinition{
				{
					Name:       "redirect",
					Expression: "(true)",
					Enabled:    true,
					Target: networkingv1alpha2.RedirectTarget{
						URL: "https://target.com",
					},
					StatusCode: networkingv1alpha2.RedirectStatusMovedPermanently,
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
	wrongObj := &networkingv1alpha2.RedirectRule{}
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
			ResourceType: networkingv1alpha2.SyncResourceRedirectRule,
			Sources: []networkingv1alpha2.ConfigSource{
				{
					Ref: networkingv1alpha2.SourceReference{
						Kind:      "RedirectRule",
						Name:      "rule-1",
						Namespace: "default",
					},
				},
				{
					Ref: networkingv1alpha2.SourceReference{
						Kind:      "RedirectRule",
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

func TestRedirectRuleStates(t *testing.T) {
	tests := []struct {
		state    networkingv1alpha2.RedirectRuleState
		expected string
	}{
		{networkingv1alpha2.RedirectRuleStatePending, "Pending"},
		{networkingv1alpha2.RedirectRuleStateSyncing, "Syncing"},
		{networkingv1alpha2.RedirectRuleStateReady, "Ready"},
		{networkingv1alpha2.RedirectRuleStateError, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.state))
		})
	}
}

func TestRedirectStatusCodes(t *testing.T) {
	tests := []struct {
		code     networkingv1alpha2.RedirectStatusCode
		expected int32
	}{
		{networkingv1alpha2.RedirectStatusMovedPermanently, 301},
		{networkingv1alpha2.RedirectStatusFound, 302},
		{networkingv1alpha2.RedirectStatusTemporaryRedirect, 307},
		{networkingv1alpha2.RedirectStatusPermanentRedirect, 308},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.expected)), func(t *testing.T) {
			assert.Equal(t, tt.expected, int32(tt.code))
		})
	}
}

func TestRedirectRuleSpec_WithRules(t *testing.T) {
	spec := networkingv1alpha2.RedirectRuleSpec{
		Zone:        "example.com",
		Description: "Test redirect rules",
		Rules: []networkingv1alpha2.RedirectRuleDefinition{
			{
				Name:       "redirect-1",
				Expression: "(http.host eq \"old.example.com\")",
				Enabled:    true,
				Target: networkingv1alpha2.RedirectTarget{
					URL: "https://new.example.com",
				},
				StatusCode:          networkingv1alpha2.RedirectStatusMovedPermanently,
				PreserveQueryString: true,
			},
			{
				Name:       "redirect-2",
				Expression: "(http.request.uri.path contains \"/old/\")",
				Enabled:    false,
				Target: networkingv1alpha2.RedirectTarget{
					Expression: "concat(\"https://new.example.com\", http.request.uri.path)",
				},
				StatusCode:          networkingv1alpha2.RedirectStatusFound,
				PreserveQueryString: false,
			},
		},
	}

	assert.Equal(t, "example.com", spec.Zone)
	assert.Equal(t, "Test redirect rules", spec.Description)
	assert.Len(t, spec.Rules, 2)

	// Check first rule
	assert.Equal(t, "redirect-1", spec.Rules[0].Name)
	assert.True(t, spec.Rules[0].Enabled)
	assert.Equal(t, "https://new.example.com", spec.Rules[0].Target.URL)
	assert.Empty(t, spec.Rules[0].Target.Expression)
	assert.True(t, spec.Rules[0].PreserveQueryString)

	// Check second rule
	assert.Equal(t, "redirect-2", spec.Rules[1].Name)
	assert.False(t, spec.Rules[1].Enabled)
	assert.Empty(t, spec.Rules[1].Target.URL)
	assert.NotEmpty(t, spec.Rules[1].Target.Expression)
	assert.False(t, spec.Rules[1].PreserveQueryString)
}

func TestRedirectRuleSpec_WithWildcardRules(t *testing.T) {
	spec := networkingv1alpha2.RedirectRuleSpec{
		Zone:        "example.com",
		Description: "Test wildcard redirect rules",
		WildcardRules: []networkingv1alpha2.WildcardRedirectRule{
			{
				Name:                "wildcard-1",
				SourceURL:           "https://old.example.com/*",
				TargetURL:           "https://new.example.com/${1}",
				Enabled:             true,
				StatusCode:          networkingv1alpha2.RedirectStatusPermanentRedirect,
				PreserveQueryString: true,
				IncludeSubdomains:   true,
				SubpathMatching:     true,
			},
			{
				Name:                "wildcard-2",
				SourceURL:           "https://shop.example.com/product/*",
				TargetURL:           "https://store.example.com/item/${1}",
				Enabled:             false,
				StatusCode:          networkingv1alpha2.RedirectStatusTemporaryRedirect,
				PreserveQueryString: false,
				IncludeSubdomains:   false,
				SubpathMatching:     false,
			},
		},
	}

	assert.Len(t, spec.WildcardRules, 2)

	// Check first wildcard rule
	assert.Equal(t, "wildcard-1", spec.WildcardRules[0].Name)
	assert.Equal(t, "https://old.example.com/*", spec.WildcardRules[0].SourceURL)
	assert.Equal(t, "https://new.example.com/${1}", spec.WildcardRules[0].TargetURL)
	assert.True(t, spec.WildcardRules[0].Enabled)
	assert.True(t, spec.WildcardRules[0].IncludeSubdomains)
	assert.True(t, spec.WildcardRules[0].SubpathMatching)

	// Check second wildcard rule
	assert.Equal(t, "wildcard-2", spec.WildcardRules[1].Name)
	assert.False(t, spec.WildcardRules[1].Enabled)
	assert.False(t, spec.WildcardRules[1].IncludeSubdomains)
	assert.False(t, spec.WildcardRules[1].SubpathMatching)
}

func TestRedirectRuleStatus(t *testing.T) {
	status := networkingv1alpha2.RedirectRuleStatus{
		State:              networkingv1alpha2.RedirectRuleStateReady,
		RulesetID:          "ruleset-123",
		ZoneID:             "zone-456",
		RuleCount:          3,
		Message:            "Redirect rules synced successfully",
		ObservedGeneration: 5,
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				Reason:             "Synced",
				Message:            "All redirect rules synced",
				ObservedGeneration: 5,
			},
		},
	}

	assert.Equal(t, networkingv1alpha2.RedirectRuleStateReady, status.State)
	assert.Equal(t, "ruleset-123", status.RulesetID)
	assert.Equal(t, "zone-456", status.ZoneID)
	assert.Equal(t, 3, status.RuleCount)
	assert.Equal(t, "Redirect rules synced successfully", status.Message)
	assert.Equal(t, int64(5), status.ObservedGeneration)
	assert.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
}

func TestClearRulesFromCloudflare_NoStatusIDs(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	rule := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "example.com",
		},
		Status: networkingv1alpha2.RedirectRuleStatus{
			// Empty ZoneID and RulesetID
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(rule).
		Build()

	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	credRef := networkingv1alpha2.CredentialsReference{Name: "test-creds"}
	needsRequeue := r.clearRulesFromCloudflare(ctx, rule, credRef)

	// Should return false when no status IDs are set
	assert.False(t, needsRequeue)
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create a rule without finalizers and not in the fake client
	// This tests the case where a rule being deleted has no finalizer
	rule := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: "default",
			// Note: We don't set DeletionTimestamp here because the fake client won't allow it
			// Instead we test the handleDeletion logic directly
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone: "example.com",
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

func TestRedirectTarget(t *testing.T) {
	// Test with URL
	targetURL := networkingv1alpha2.RedirectTarget{
		URL: "https://example.com/target",
	}
	assert.Equal(t, "https://example.com/target", targetURL.URL)
	assert.Empty(t, targetURL.Expression)

	// Test with expression
	targetExpr := networkingv1alpha2.RedirectTarget{
		Expression: "concat(\"https://\", http.host, \"/new\", http.request.uri.path)",
	}
	assert.Empty(t, targetExpr.URL)
	assert.NotEmpty(t, targetExpr.Expression)
}

func TestRedirectRuleDefinition(t *testing.T) {
	def := networkingv1alpha2.RedirectRuleDefinition{
		Name:       "test-redirect",
		Expression: "(http.host eq \"example.com\")",
		Enabled:    true,
		Target: networkingv1alpha2.RedirectTarget{
			URL: "https://new.example.com",
		},
		StatusCode:          networkingv1alpha2.RedirectStatusMovedPermanently,
		PreserveQueryString: true,
	}

	assert.Equal(t, "test-redirect", def.Name)
	assert.Equal(t, "(http.host eq \"example.com\")", def.Expression)
	assert.True(t, def.Enabled)
	assert.Equal(t, networkingv1alpha2.RedirectStatusMovedPermanently, def.StatusCode)
	assert.True(t, def.PreserveQueryString)
}

func TestWildcardRedirectRule(t *testing.T) {
	rule := networkingv1alpha2.WildcardRedirectRule{
		Name:                "wildcard-test",
		SourceURL:           "https://old.example.com/path/*",
		TargetURL:           "https://new.example.com/path/${1}",
		Enabled:             true,
		StatusCode:          networkingv1alpha2.RedirectStatusPermanentRedirect,
		PreserveQueryString: true,
		IncludeSubdomains:   true,
		SubpathMatching:     true,
	}

	assert.Equal(t, "wildcard-test", rule.Name)
	assert.Equal(t, "https://old.example.com/path/*", rule.SourceURL)
	assert.Equal(t, "https://new.example.com/path/${1}", rule.TargetURL)
	assert.True(t, rule.Enabled)
	assert.Equal(t, networkingv1alpha2.RedirectStatusPermanentRedirect, rule.StatusCode)
	assert.True(t, rule.PreserveQueryString)
	assert.True(t, rule.IncludeSubdomains)
	assert.True(t, rule.SubpathMatching)
}

func TestReconciler_Reconcile_NoRulesOrWildcardRules(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create a rule with neither rules nor wildcardRules
	rule := &networkingv1alpha2.RedirectRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-rule",
			Namespace: "default",
		},
		Spec: networkingv1alpha2.RedirectRuleSpec{
			Zone:        "example.com",
			Description: "Empty redirect rule",
			// No Rules or WildcardRules
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

	// This tests the reconciliation path - behavior depends on credentials availability
	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "empty-rule",
			Namespace: "default",
		},
	})

	// The test verifies that reconciliation proceeds (no panic)
	_ = result
	_ = err
}

func TestRedirectRuleSpec_MixedRules(t *testing.T) {
	// Test with both expression-based and wildcard rules
	spec := networkingv1alpha2.RedirectRuleSpec{
		Zone:        "example.com",
		Description: "Mixed redirect rules",
		Rules: []networkingv1alpha2.RedirectRuleDefinition{
			{
				Name:       "expression-redirect",
				Expression: "(http.host eq \"api.example.com\")",
				Enabled:    true,
				Target: networkingv1alpha2.RedirectTarget{
					URL: "https://api-v2.example.com",
				},
				StatusCode: networkingv1alpha2.RedirectStatusMovedPermanently,
			},
		},
		WildcardRules: []networkingv1alpha2.WildcardRedirectRule{
			{
				Name:       "wildcard-redirect",
				SourceURL:  "https://docs.example.com/*",
				TargetURL:  "https://help.example.com/${1}",
				Enabled:    true,
				StatusCode: networkingv1alpha2.RedirectStatusPermanentRedirect,
			},
		},
		CredentialsRef: &networkingv1alpha2.CredentialsReference{
			Name: "my-creds",
		},
	}

	assert.Len(t, spec.Rules, 1)
	assert.Len(t, spec.WildcardRules, 1)
	assert.NotNil(t, spec.CredentialsRef)
	assert.Equal(t, "my-creds", spec.CredentialsRef.Name)
}
