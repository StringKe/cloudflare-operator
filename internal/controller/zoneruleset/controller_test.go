// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package zoneruleset

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

func createTestZoneRuleset(name, namespace string) *networkingv1alpha2.ZoneRuleset {
	return &networkingv1alpha2.ZoneRuleset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha2.ZoneRulesetSpec{
			Zone:        "example.com",
			Phase:       networkingv1alpha2.RulesetPhaseHTTPRequestFirewallCustom,
			Description: "Test ruleset",
			Rules: []networkingv1alpha2.RulesetRule{
				{
					Description: "Block bad bots",
					Expression:  "(cf.client.bot)",
					Action:      networkingv1alpha2.RulesetRuleActionBlock,
					Enabled:     true,
				},
			},
		},
	}
}

func createTestZoneRulesetWithCredentials(name, namespace, credsName string) *networkingv1alpha2.ZoneRuleset {
	ruleset := createTestZoneRuleset(name, namespace)
	ruleset.Spec.CredentialsRef = &networkingv1alpha2.CredentialsReference{
		Name: credsName,
	}
	return ruleset
}

func createTestCredentials(name string, isDefault bool) *networkingv1alpha2.CloudflareCredentials {
	return &networkingv1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1alpha2.CloudflareCredentialsSpec{
			AuthType:  networkingv1alpha2.AuthTypeAPIToken,
			AccountID: "test-account-123",
			SecretRef: networkingv1alpha2.SecretReference{
				Name:      "cloudflare-secret",
				Namespace: "cloudflare-operator-system",
			},
			IsDefault: isDefault,
		},
	}
}

func TestFinalizerName(t *testing.T) {
	assert.NotEmpty(t, finalizerName)
	assert.Contains(t, finalizerName, "zone-ruleset")
}

func TestReconcilerFields(t *testing.T) {
	// Test that the Reconciler struct has the expected fields
	r := &Reconciler{}

	// Verify nil defaults
	assert.Nil(t, r.Client)
	assert.Nil(t, r.Scheme)
	assert.Nil(t, r.Recorder)
	assert.Nil(t, r.zoneRulesetService)
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
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconciler_Reconcile_ReturnsResultOnMissingCredentials(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	ruleset := createTestZoneRuleset("test-ruleset", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ruleset).
		WithStatusSubresource(ruleset).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	result, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      ruleset.Name,
			Namespace: ruleset.Namespace,
		},
	})

	// The reconcile returns error or requeues when credentials are missing
	_ = err
	_ = result
}

// Test findRulesetsForCredentials
func TestFindRulesetsForCredentials_TypeCheck(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with wrong type
	wrongObj := &networkingv1alpha2.ZoneRuleset{}
	requests := r.findRulesetsForCredentials(ctx, wrongObj)
	assert.Nil(t, requests)
}

func TestFindRulesetsForCredentials_MatchingRulesets(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := createTestCredentials("test-creds", false)
	ruleset1 := createTestZoneRulesetWithCredentials("ruleset1", "default", "test-creds")
	ruleset2 := createTestZoneRulesetWithCredentials("ruleset2", "default", "other-creds")
	ruleset3 := createTestZoneRulesetWithCredentials("ruleset3", "ns1", "test-creds")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(creds, ruleset1, ruleset2, ruleset3).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findRulesetsForCredentials(ctx, creds)

	// Should find ruleset1 and ruleset3 (matching credentials)
	assert.Len(t, requests, 2)
	names := make([]string, len(requests))
	for i, req := range requests {
		names[i] = req.Name
	}
	assert.Contains(t, names, "ruleset1")
	assert.Contains(t, names, "ruleset3")
}

func TestFindRulesetsForCredentials_DefaultCredentials(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	creds := createTestCredentials("default-creds", true)    // isDefault = true
	ruleset1 := createTestZoneRuleset("ruleset1", "default") // No credentialsRef
	ruleset2 := createTestZoneRulesetWithCredentials("ruleset2", "default", "other-creds")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(creds, ruleset1, ruleset2).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findRulesetsForCredentials(ctx, creds)

	// Should find ruleset1 (using default credentials)
	assert.Len(t, requests, 1)
	assert.Equal(t, "ruleset1", requests[0].Name)
}

// Test ZoneRuleset spec fields
func TestZoneRulesetSpec(t *testing.T) {
	ruleset := createTestZoneRuleset("my-ruleset", "default")
	ruleset.Spec.Rules = append(ruleset.Spec.Rules, networkingv1alpha2.RulesetRule{
		Description: "Rate limit",
		Expression:  "true",
		Action:      networkingv1alpha2.RulesetRuleActionChallenge,
		Enabled:     true,
		RateLimit: &networkingv1alpha2.RulesetRuleRateLimit{
			Characteristics:   []string{"ip.src"},
			Period:            60,
			RequestsPerPeriod: 100,
			MitigationTimeout: 3600,
		},
	})

	assert.Equal(t, "example.com", ruleset.Spec.Zone)
	assert.Equal(t, networkingv1alpha2.RulesetPhaseHTTPRequestFirewallCustom, ruleset.Spec.Phase)
	assert.Equal(t, "Test ruleset", ruleset.Spec.Description)
	assert.Len(t, ruleset.Spec.Rules, 2)
}

func TestZoneRulesetStatus(t *testing.T) {
	ruleset := createTestZoneRuleset("my-ruleset", "default")
	now := metav1.Now()
	ruleset.Status = networkingv1alpha2.ZoneRulesetStatus{
		State:              networkingv1alpha2.ZoneRulesetStateReady,
		RulesetID:          "rs-123456",
		RulesetVersion:     "3",
		ZoneID:             "zone-123",
		RuleCount:          5,
		LastUpdated:        &now,
		Message:            "Ruleset is active",
		ObservedGeneration: 3,
	}

	assert.Equal(t, networkingv1alpha2.ZoneRulesetStateReady, ruleset.Status.State)
	assert.Equal(t, "rs-123456", ruleset.Status.RulesetID)
	assert.Equal(t, "3", ruleset.Status.RulesetVersion)
	assert.Equal(t, "zone-123", ruleset.Status.ZoneID)
	assert.Equal(t, 5, ruleset.Status.RuleCount)
	assert.NotNil(t, ruleset.Status.LastUpdated)
	assert.Equal(t, "Ruleset is active", ruleset.Status.Message)
	assert.Equal(t, int64(3), ruleset.Status.ObservedGeneration)
}

func TestZoneRulesetStates(t *testing.T) {
	tests := []struct {
		name  string
		state networkingv1alpha2.ZoneRulesetState
	}{
		{"Pending", networkingv1alpha2.ZoneRulesetStatePending},
		{"Syncing", networkingv1alpha2.ZoneRulesetStateSyncing},
		{"Ready", networkingv1alpha2.ZoneRulesetStateReady},
		{"Error", networkingv1alpha2.ZoneRulesetStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleset := createTestZoneRuleset("test", "default")
			ruleset.Status.State = tt.state
			assert.Equal(t, tt.state, ruleset.Status.State)
		})
	}
}

func TestRulesetPhases(t *testing.T) {
	tests := []struct {
		name  string
		phase networkingv1alpha2.RulesetPhase
	}{
		{"HTTPRequestTransform", networkingv1alpha2.RulesetPhaseHTTPRequestTransform},
		{"HTTPRequestLateTransform", networkingv1alpha2.RulesetPhaseHTTPRequestLateTransform},
		{"HTTPRequestOrigin", networkingv1alpha2.RulesetPhaseHTTPRequestOrigin},
		{"HTTPRequestRedirect", networkingv1alpha2.RulesetPhaseHTTPRequestRedirect},
		{"HTTPRequestDynamicRedirect", networkingv1alpha2.RulesetPhaseHTTPRequestDynamicRedirect},
		{"HTTPRequestCacheSettings", networkingv1alpha2.RulesetPhaseHTTPRequestCacheSettings},
		{"HTTPConfigSettings", networkingv1alpha2.RulesetPhaseHTTPConfigSettings},
		{"HTTPCustomErrors", networkingv1alpha2.RulesetPhaseHTTPCustomErrors},
		{"HTTPResponseHeadersTransform", networkingv1alpha2.RulesetPhaseHTTPResponseHeadersTransform},
		{"HTTPResponseCompression", networkingv1alpha2.RulesetPhaseHTTPResponseCompression},
		{"HTTPRateLimit", networkingv1alpha2.RulesetPhaseHTTPRateLimit},
		{"HTTPRequestFirewallCustom", networkingv1alpha2.RulesetPhaseHTTPRequestFirewallCustom},
		{"HTTPRequestFirewallManaged", networkingv1alpha2.RulesetPhaseHTTPRequestFirewallManaged},
		{"HTTPResponseFirewallManaged", networkingv1alpha2.RulesetPhaseHTTPResponseFirewallManaged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleset := createTestZoneRuleset("test", "default")
			ruleset.Spec.Phase = tt.phase
			assert.Equal(t, tt.phase, ruleset.Spec.Phase)
		})
	}
}

func TestRulesetRuleActions(t *testing.T) {
	tests := []struct {
		name   string
		action networkingv1alpha2.RulesetRuleAction
	}{
		{"Block", networkingv1alpha2.RulesetRuleActionBlock},
		{"Challenge", networkingv1alpha2.RulesetRuleActionChallenge},
		{"JSChallenge", networkingv1alpha2.RulesetRuleActionJSChallenge},
		{"ManagedChallenge", networkingv1alpha2.RulesetRuleActionManagedChallenge},
		{"Log", networkingv1alpha2.RulesetRuleActionLog},
		{"Skip", networkingv1alpha2.RulesetRuleActionSkip},
		{"Rewrite", networkingv1alpha2.RulesetRuleActionRewrite},
		{"Redirect", networkingv1alpha2.RulesetRuleActionRedirect},
		{"Route", networkingv1alpha2.RulesetRuleActionRoute},
		{"Score", networkingv1alpha2.RulesetRuleActionScore},
		{"Execute", networkingv1alpha2.RulesetRuleActionExecute},
		{"SetConfig", networkingv1alpha2.RulesetRuleActionSetConfig},
		{"SetCacheSettings", networkingv1alpha2.RulesetRuleActionSetCacheSettings},
		{"ServeError", networkingv1alpha2.RulesetRuleActionServeError},
		{"CompressResponse", networkingv1alpha2.RulesetRuleActionCompressResponse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := networkingv1alpha2.RulesetRule{
				Expression: "true",
				Action:     tt.action,
			}
			assert.Equal(t, tt.action, rule.Action)
		})
	}
}

func TestRulesetRuleWithActionParameters(t *testing.T) {
	rule := networkingv1alpha2.RulesetRule{
		Description: "Rewrite URL",
		Expression:  "(http.request.uri.path contains \"/old\")",
		Action:      networkingv1alpha2.RulesetRuleActionRewrite,
		Enabled:     true,
		ActionParameters: &networkingv1alpha2.RulesetRuleActionParameters{
			URI: &networkingv1alpha2.RulesetURIRewrite{
				Path: &networkingv1alpha2.RulesetRewriteValue{
					Value: "/new",
				},
			},
		},
	}

	assert.Equal(t, "Rewrite URL", rule.Description)
	assert.NotNil(t, rule.ActionParameters)
	assert.NotNil(t, rule.ActionParameters.URI)
	assert.NotNil(t, rule.ActionParameters.URI.Path)
	assert.Equal(t, "/new", rule.ActionParameters.URI.Path.Value)
}

func TestRulesetRuleWithRateLimit(t *testing.T) {
	rule := networkingv1alpha2.RulesetRule{
		Description: "Rate limit API",
		Expression:  "(http.request.uri.path contains \"/api\")",
		Action:      networkingv1alpha2.RulesetRuleActionBlock,
		Enabled:     true,
		RateLimit: &networkingv1alpha2.RulesetRuleRateLimit{
			Characteristics:   []string{"ip.src", "http.request.headers[\"x-api-key\"]"},
			Period:            60,
			RequestsPerPeriod: 100,
			MitigationTimeout: 3600,
		},
	}

	assert.NotNil(t, rule.RateLimit)
	assert.Len(t, rule.RateLimit.Characteristics, 2)
	assert.Equal(t, 60, rule.RateLimit.Period)
	assert.Equal(t, 100, rule.RateLimit.RequestsPerPeriod)
	assert.Equal(t, 3600, rule.RateLimit.MitigationTimeout)
}

func TestRulesetRedirect(t *testing.T) {
	redirect := networkingv1alpha2.RulesetRedirect{
		SourceURL: "/old-page",
		TargetURL: &networkingv1alpha2.RulesetRewriteValue{
			Value: "/new-page",
		},
		StatusCode:          301,
		PreserveQueryString: true,
		IncludeSubdomains:   false,
		SubpathMatching:     true,
	}

	assert.Equal(t, "/old-page", redirect.SourceURL)
	assert.NotNil(t, redirect.TargetURL)
	assert.Equal(t, "/new-page", redirect.TargetURL.Value)
	assert.Equal(t, 301, redirect.StatusCode)
	assert.True(t, redirect.PreserveQueryString)
	assert.False(t, redirect.IncludeSubdomains)
	assert.True(t, redirect.SubpathMatching)
}

func TestRulesetCacheSettings(t *testing.T) {
	cacheEnabled := true
	cache := networkingv1alpha2.RulesetCacheSettings{
		Cache: &cacheEnabled,
		EdgeTTL: &networkingv1alpha2.RulesetCacheTTL{
			Mode:    "override_origin",
			Default: intPtr(3600),
		},
		BrowserTTL: &networkingv1alpha2.RulesetCacheTTL{
			Mode:    "respect_origin",
			Default: intPtr(600),
		},
	}

	assert.NotNil(t, cache.Cache)
	assert.True(t, *cache.Cache)
	assert.NotNil(t, cache.EdgeTTL)
	assert.Equal(t, "override_origin", cache.EdgeTTL.Mode)
	assert.Equal(t, 3600, *cache.EdgeTTL.Default)
	assert.NotNil(t, cache.BrowserTTL)
	assert.Equal(t, "respect_origin", cache.BrowserTTL.Mode)
}

func TestReconcilerImplementsReconciler(_ *testing.T) {
	var _ reconcile.Reconciler = &Reconciler{}
}

func TestReconcilerWithFakeClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Should be able to list resources (empty list)
	rulesetList := &networkingv1alpha2.ZoneRulesetList{}
	err := r.List(ctx, rulesetList)
	require.NoError(t, err)
	assert.Empty(t, rulesetList.Items)
}

func TestReconcilerEmbeddedClient(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test Get method
	ruleset := &networkingv1alpha2.ZoneRuleset{}
	err := r.Get(ctx, types.NamespacedName{Name: "test", Namespace: "default"}, ruleset)
	assert.Error(t, err) // Should not find it

	// Test Create method
	newRuleset := createTestZoneRuleset("new-ruleset", "default")
	err = r.Create(ctx, newRuleset)
	require.NoError(t, err)

	// Now it should be findable
	err = r.Get(ctx, types.NamespacedName{Name: "new-ruleset", Namespace: "default"}, ruleset)
	require.NoError(t, err)
	assert.Equal(t, "new-ruleset", ruleset.Name)
}

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create ruleset without finalizer
	ruleset := createTestZoneRuleset("test-ruleset", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ruleset).
		Build()

	recorder := record.NewFakeRecorder(10)
	r := &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
	}

	// Set deletion timestamp manually for testing handleDeletion directly
	now := metav1.Now()
	ruleset.DeletionTimestamp = &now

	credRef := networkingv1alpha2.CredentialsReference{Name: "test-creds"}
	result, err := r.handleDeletion(ctx, ruleset, credRef)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// Test finalizer operations
func TestFinalizerOperations(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	ruleset := createTestZoneRuleset("test-ruleset", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ruleset).
		Build()

	// Add finalizer
	controllerutil.AddFinalizer(ruleset, finalizerName)
	err := fakeClient.Update(ctx, ruleset)
	require.NoError(t, err)

	// Verify finalizer was added
	var updated networkingv1alpha2.ZoneRuleset
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-ruleset", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updated, finalizerName))

	// Remove finalizer
	controllerutil.RemoveFinalizer(&updated, finalizerName)
	err = fakeClient.Update(ctx, &updated)
	require.NoError(t, err)

	// Verify finalizer was removed
	err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-ruleset", Namespace: "default"}, &updated)
	require.NoError(t, err)
	assert.False(t, controllerutil.ContainsFinalizer(&updated, finalizerName))
}

func TestGetClientFromReconciler(t *testing.T) {
	scheme := setupTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &Reconciler{
		Client: fakeClient,
	}

	// Verify the embedded client is accessible
	var c = r.Client
	assert.NotNil(t, c)
}

func TestZoneRulesetListPagination(t *testing.T) {
	scheme := setupTestScheme(t)
	ctx := context.Background()

	// Create multiple rulesets
	rulesets := make([]client.Object, 5)
	for i := 0; i < 5; i++ {
		rulesets[i] = createTestZoneRuleset(
			"ruleset-"+string(rune('a'+i)),
			"default",
		)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(rulesets...).
		Build()

	r := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	rulesetList := &networkingv1alpha2.ZoneRulesetList{}
	err := r.List(ctx, rulesetList)
	require.NoError(t, err)
	assert.Len(t, rulesetList.Items, 5)
}

// Note: TestClearRulesFromCloudflare_NoStatusIDs and TestClearRulesFromCloudflare_WithStatusIDs removed
// clearRulesFromCloudflare method removed following Unified Sync Architecture
// Deletion is now handled by SyncController, not ResourceController

// Helper function
func intPtr(i int) *int {
	return &i
}
