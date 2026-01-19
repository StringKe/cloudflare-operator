// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package accessapplication

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	accesssvc "github.com/StringKe/cloudflare-operator/internal/service/access"
)

func init() {
	ctrllog.SetLogger(zap.New(zap.UseDevMode(true)))
}

func setupReconciler(t *testing.T, objs ...runtime.Object) *Reconciler {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1alpha2.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	return &Reconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
		ctx:      context.Background(),
		log:      ctrllog.Log.WithName("test"),
	}
}

// TestResolvePoliciesInlineRulesMode tests that policies with inline rules are resolved correctly.
//
//nolint:revive // Test table-driven tests naturally have high cognitive complexity
func TestResolvePoliciesInlineRulesMode(t *testing.T) {
	tests := []struct {
		name        string
		policies    []networkingv1alpha2.AccessPolicyRef
		wantCount   int
		wantError   bool
		description string
	}{
		{
			name:        "empty policies",
			policies:    nil,
			wantCount:   0,
			wantError:   false,
			description: "Empty policies should return empty slice",
		},
		{
			name: "single policy with email include rule",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							Email: &networkingv1alpha2.AccessGroupEmailRule{
								Email: "admin@example.com",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Single policy with email include rule",
		},
		{
			name: "policy with all rule types (include, exclude, require)",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision:   "allow",
					Precedence: 1,
					PolicyName: "Full Policy",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							EmailDomain: &networkingv1alpha2.AccessGroupEmailDomainRule{
								Domain: "example.com",
							},
						},
					},
					Exclude: []networkingv1alpha2.AccessGroupRule{
						{
							Email: &networkingv1alpha2.AccessGroupEmailRule{
								Email: "blocked@example.com",
							},
						},
					},
					Require: []networkingv1alpha2.AccessGroupRule{
						{
							IPRanges: &networkingv1alpha2.AccessGroupIPRangesRule{
								IP: []string{"10.0.0.0/8"},
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with include, exclude, and require rules",
		},
		{
			name: "multiple policies with inline rules",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision:   "allow",
					Precedence: 1,
					Include: []networkingv1alpha2.AccessGroupRule{
						{Everyone: true},
					},
				},
				{
					Decision:   "deny",
					Precedence: 2,
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							Country: &networkingv1alpha2.AccessGroupCountryRule{
								Country: []string{"CN"},
							},
						},
					},
				},
			},
			wantCount:   2,
			wantError:   false,
			description: "Multiple policies with different rule types",
		},
		{
			name: "policy with service token rule",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "non_identity",
					Include: []networkingv1alpha2.AccessGroupRule{
						{AnyValidServiceToken: true},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with service token rule for non_identity decision",
		},
		{
			name: "policy with certificate rule",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{Certificate: true},
					},
					Require: []networkingv1alpha2.AccessGroupRule{
						{
							CommonName: &networkingv1alpha2.AccessGroupCommonNameRule{
								CommonName: "client.example.com",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with certificate and common name rules",
		},
		{
			name: "policy with GSuite and GitHub rules",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							GSuite: &networkingv1alpha2.AccessGroupGSuiteRule{
								IdentityProviderID: "gsuite-idp-id",
							},
						},
						{
							GitHub: &networkingv1alpha2.AccessGroupGitHubRule{
								Name:               "myorg",
								IdentityProviderID: "github-idp-id",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with IdP-specific rules (GSuite, GitHub)",
		},
		{
			name: "policy with Azure and Okta rules",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							Azure: &networkingv1alpha2.AccessGroupAzureRule{
								ID:                 "azure-group-id",
								IdentityProviderID: "azure-idp-id",
							},
						},
						{
							Okta: &networkingv1alpha2.AccessGroupOktaRule{
								Name:               "okta-group",
								IdentityProviderID: "okta-idp-id",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with Azure AD and Okta group rules",
		},
		{
			name: "policy with SAML and OIDC rules",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							SAML: &networkingv1alpha2.AccessGroupSAMLRule{
								AttributeName:      "department",
								AttributeValue:     "engineering",
								IdentityProviderID: "saml-idp-id",
							},
						},
						{
							OIDC: &networkingv1alpha2.AccessGroupOIDCRule{
								ClaimName:          "groups",
								ClaimValue:         "admin",
								IdentityProviderID: "oidc-idp-id",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with SAML and OIDC claim rules",
		},
		{
			name: "policy with device posture rule",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							DevicePosture: &networkingv1alpha2.AccessGroupDevicePostureRule{
								IntegrationUID: "posture-integration-id",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with device posture rule",
		},
		{
			name: "policy with auth method and external evaluation",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							AuthMethod: &networkingv1alpha2.AccessGroupAuthMethodRule{
								AuthMethod: "mfa",
							},
						},
					},
					Require: []networkingv1alpha2.AccessGroupRule{
						{
							ExternalEvaluation: &networkingv1alpha2.AccessGroupExternalEvaluationRule{
								EvaluateURL: "https://eval.example.com/check",
								KeysURL:     "https://eval.example.com/.well-known/jwks.json",
							},
						},
					},
				},
			},
			wantCount:   1,
			wantError:   false,
			description: "Policy with auth method and external evaluation rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test application with policies
			app := &networkingv1alpha2.AccessApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: networkingv1alpha2.AccessApplicationSpec{
					Name:     "Test Application",
					Domain:   "app.example.com",
					Type:     "self_hosted",
					Policies: tt.policies,
				},
			}

			r := setupReconciler(t)
			r.app = app

			policies, err := r.resolvePolicies()

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, policies, tt.wantCount, tt.description)

				// Verify that inline rules are properly transferred
				for i, policy := range policies {
					if i < len(tt.policies) {
						srcPolicy := tt.policies[i]

						// Check that inline rules are preserved
						if len(srcPolicy.Include) > 0 {
							assert.Len(t, policy.Include, len(srcPolicy.Include), "Include rules count mismatch")
						}
						if len(srcPolicy.Exclude) > 0 {
							assert.Len(t, policy.Exclude, len(srcPolicy.Exclude), "Exclude rules count mismatch")
						}
						if len(srcPolicy.Require) > 0 {
							assert.Len(t, policy.Require, len(srcPolicy.Require), "Require rules count mismatch")
						}
					}
				}
			}
		})
	}
}

// TestResolvePoliciesGroupReferenceMode tests that policies with group references are resolved correctly.
//
//nolint:revive // Test table-driven tests naturally have high cognitive complexity
func TestResolvePoliciesGroupReferenceMode(t *testing.T) {
	tests := []struct {
		name         string
		policies     []networkingv1alpha2.AccessPolicyRef
		accessGroups []*networkingv1alpha2.AccessGroup
		wantCount    int
		wantGroupIDs []string
		wantError    bool
		description  string
	}{
		{
			name: "policy with direct Cloudflare group ID",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					GroupID:  "cf-group-id-12345",
				},
			},
			wantCount:    1,
			wantGroupIDs: []string{""}, // CloudflareGroupID is passed as reference, not resolved here
			wantError:    false,
			description:  "Direct Cloudflare group ID reference",
		},
		{
			name: "policy with Cloudflare group name",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision:            "allow",
					CloudflareGroupName: "My CF Group",
				},
			},
			wantCount:    1,
			wantGroupIDs: []string{""}, // CloudflareGroupName is passed as reference, not resolved here
			wantError:    false,
			description:  "Cloudflare group name reference",
		},
		{
			name: "policy with K8s AccessGroup reference - already synced",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Name:     "my-k8s-group",
				},
			},
			accessGroups: []*networkingv1alpha2.AccessGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-k8s-group",
					},
					Spec: networkingv1alpha2.AccessGroupSpec{
						Name: "My K8s Group",
					},
					Status: networkingv1alpha2.AccessGroupStatus{
						GroupID: "resolved-group-id",
					},
				},
			},
			wantCount:    1,
			wantGroupIDs: []string{"resolved-group-id"},
			wantError:    false,
			description:  "K8s AccessGroup reference with synced status",
		},
		{
			name: "policy with K8s AccessGroup reference - not yet synced",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Name:     "my-pending-group",
				},
			},
			accessGroups: []*networkingv1alpha2.AccessGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-pending-group",
					},
					Spec: networkingv1alpha2.AccessGroupSpec{
						Name: "My Pending Group",
					},
					Status: networkingv1alpha2.AccessGroupStatus{
						GroupID: "", // Not yet synced
					},
				},
			},
			wantCount:    1,
			wantGroupIDs: []string{""}, // K8sAccessGroupName passed as reference
			wantError:    false,
			description:  "K8s AccessGroup reference without GroupID in status",
		},
		{
			name: "policy with K8s AccessGroup reference - not found",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Name:     "non-existent-group",
				},
			},
			accessGroups: nil,
			wantCount:    0, // Should be skipped due to not found
			wantError:    false,
			description:  "K8s AccessGroup not found",
		},
		{
			name: "policy with no reference (empty)",
			policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					// No Name, GroupID, or CloudflareGroupName
				},
			},
			wantCount:   0, // Should be skipped
			wantError:   false,
			description: "Empty policy reference is skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare runtime objects
			objs := make([]runtime.Object, 0, len(tt.accessGroups))
			for _, ag := range tt.accessGroups {
				objs = append(objs, ag)
			}

			// Create test application with policies
			app := &networkingv1alpha2.AccessApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-app",
				},
				Spec: networkingv1alpha2.AccessApplicationSpec{
					Name:     "Test Application",
					Domain:   "app.example.com",
					Type:     "self_hosted",
					Policies: tt.policies,
				},
			}

			r := setupReconciler(t, objs...)
			r.app = app

			policies, err := r.resolvePolicies()

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, policies, tt.wantCount, tt.description)

				// Check resolved group IDs
				for i, policy := range policies {
					if i < len(tt.wantGroupIDs) {
						assert.Equal(t, tt.wantGroupIDs[i], policy.GroupID, "GroupID mismatch at index %d", i)
					}
				}
			}
		})
	}
}

// TestResolvePoliciesMixedMode tests that policies with both inline rules and group references work together.
func TestResolvePoliciesMixedMode(t *testing.T) {
	// Create an AccessGroup for reference
	accessGroup := &networkingv1alpha2.AccessGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "admin-group",
		},
		Spec: networkingv1alpha2.AccessGroupSpec{
			Name: "Administrators",
		},
		Status: networkingv1alpha2.AccessGroupStatus{
			GroupID: "admin-group-id-12345",
		},
	}

	// Create test application with mixed policies
	app := &networkingv1alpha2.AccessApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mixed-policy-app",
		},
		Spec: networkingv1alpha2.AccessApplicationSpec{
			Name:   "Mixed Policy Application",
			Domain: "app.example.com",
			Type:   "self_hosted",
			Policies: []networkingv1alpha2.AccessPolicyRef{
				// Policy 1: Inline rules for employees
				{
					Decision:   "allow",
					Precedence: 1,
					PolicyName: "Employee Access",
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							EmailDomain: &networkingv1alpha2.AccessGroupEmailDomainRule{
								Domain: "company.com",
							},
						},
					},
					Require: []networkingv1alpha2.AccessGroupRule{
						{
							DevicePosture: &networkingv1alpha2.AccessGroupDevicePostureRule{
								IntegrationUID: "managed-device-check",
							},
						},
					},
				},
				// Policy 2: Group reference for admins
				{
					Decision:   "allow",
					Precedence: 2,
					Name:       "admin-group",
				},
				// Policy 3: Inline rules for service accounts
				{
					Decision:   "non_identity",
					Precedence: 3,
					PolicyName: "Service Account Access",
					Include: []networkingv1alpha2.AccessGroupRule{
						{AnyValidServiceToken: true},
					},
				},
				// Policy 4: Direct Cloudflare group ID
				{
					Decision:   "deny",
					Precedence: 4,
					GroupID:    "blocked-users-group-id",
				},
			},
		},
	}

	r := setupReconciler(t, accessGroup)
	r.app = app

	policies, err := r.resolvePolicies()

	assert.NoError(t, err)
	assert.Len(t, policies, 4, "Should resolve 4 policies")

	// Policy 1: Check inline rules
	assert.True(t, policies[0].HasInlineRules(), "Policy 1 should have inline rules")
	assert.Len(t, policies[0].Include, 1)
	assert.Len(t, policies[0].Require, 1)
	assert.Equal(t, "Employee Access", policies[0].PolicyName)
	assert.Equal(t, 1, policies[0].Precedence)

	// Policy 2: Check K8s AccessGroup reference
	assert.False(t, policies[1].HasInlineRules(), "Policy 2 should not have inline rules")
	assert.Equal(t, "admin-group-id-12345", policies[1].GroupID)
	assert.Equal(t, 2, policies[1].Precedence)

	// Policy 3: Check inline rules for service accounts
	assert.True(t, policies[2].HasInlineRules(), "Policy 3 should have inline rules")
	assert.Equal(t, "non_identity", policies[2].Decision)
	assert.Equal(t, "Service Account Access", policies[2].PolicyName)
	assert.Equal(t, 3, policies[2].Precedence)

	// Policy 4: Check Cloudflare group ID reference
	assert.False(t, policies[3].HasInlineRules(), "Policy 4 should not have inline rules")
	assert.Equal(t, "blocked-users-group-id", policies[3].CloudflareGroupID)
	assert.Equal(t, "deny", policies[3].Decision)
	assert.Equal(t, 4, policies[3].Precedence)
}

// TestResolvePoliciesPrecedenceAutomaticAssignment tests that precedence is auto-assigned when not specified.
func TestResolvePoliciesPrecedenceAutomaticAssignment(t *testing.T) {
	app := &networkingv1alpha2.AccessApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "auto-precedence-app",
		},
		Spec: networkingv1alpha2.AccessApplicationSpec{
			Name:   "Auto Precedence Application",
			Domain: "app.example.com",
			Type:   "self_hosted",
			Policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					// No precedence specified
					Include: []networkingv1alpha2.AccessGroupRule{
						{Everyone: true},
					},
				},
				{
					Decision: "allow",
					// No precedence specified
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							Email: &networkingv1alpha2.AccessGroupEmailRule{
								Email: "vip@example.com",
							},
						},
					},
				},
				{
					Decision:   "deny",
					Precedence: 100, // Explicit precedence
					Include: []networkingv1alpha2.AccessGroupRule{
						{
							Country: &networkingv1alpha2.AccessGroupCountryRule{
								Country: []string{"RU"},
							},
						},
					},
				},
			},
		},
	}

	r := setupReconciler(t)
	r.app = app

	policies, err := r.resolvePolicies()

	assert.NoError(t, err)
	assert.Len(t, policies, 3)

	// Check auto-assigned precedence (1-indexed based on position)
	assert.Equal(t, 1, policies[0].Precedence, "First policy should have precedence 1")
	assert.Equal(t, 2, policies[1].Precedence, "Second policy should have precedence 2")
	assert.Equal(t, 100, policies[2].Precedence, "Third policy should keep explicit precedence 100")
}

// TestResolvePoliciesDefaultDecision tests that decision defaults to "allow" when not specified.
func TestResolvePoliciesDefaultDecision(t *testing.T) {
	app := &networkingv1alpha2.AccessApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-decision-app",
		},
		Spec: networkingv1alpha2.AccessApplicationSpec{
			Name:   "Default Decision Application",
			Domain: "app.example.com",
			Type:   "self_hosted",
			Policies: []networkingv1alpha2.AccessPolicyRef{
				{
					// No decision specified
					Include: []networkingv1alpha2.AccessGroupRule{
						{Everyone: true},
					},
				},
			},
		},
	}

	r := setupReconciler(t)
	r.app = app

	policies, err := r.resolvePolicies()

	assert.NoError(t, err)
	assert.Len(t, policies, 1)
	assert.Equal(t, "allow", policies[0].Decision, "Decision should default to 'allow'")
}

// TestResolvePoliciesAllRuleTypes tests that all AccessGroupRule types are properly transferred.
func TestResolvePoliciesAllRuleTypes(t *testing.T) {
	app := &networkingv1alpha2.AccessApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "all-rules-app",
		},
		Spec: networkingv1alpha2.AccessApplicationSpec{
			Name:   "All Rules Application",
			Domain: "app.example.com",
			Type:   "self_hosted",
			Policies: []networkingv1alpha2.AccessPolicyRef{
				{
					Decision: "allow",
					Include: []networkingv1alpha2.AccessGroupRule{
						{Email: &networkingv1alpha2.AccessGroupEmailRule{Email: "user@example.com"}},
						{EmailDomain: &networkingv1alpha2.AccessGroupEmailDomainRule{Domain: "example.com"}},
						{EmailList: &networkingv1alpha2.AccessGroupEmailListRule{ID: "email-list-id"}},
						{Everyone: true},
						{IPRanges: &networkingv1alpha2.AccessGroupIPRangesRule{IP: []string{"10.0.0.0/8"}}},
						{IPList: &networkingv1alpha2.AccessGroupIPListRule{ID: "ip-list-id"}},
						{Country: &networkingv1alpha2.AccessGroupCountryRule{Country: []string{"US"}}},
						{Group: &networkingv1alpha2.AccessGroupGroupRule{ID: "nested-group-id"}},
						{ServiceToken: &networkingv1alpha2.AccessGroupServiceTokenRule{TokenID: "token-id"}},
						{AnyValidServiceToken: true},
						{Certificate: true},
						{CommonName: &networkingv1alpha2.AccessGroupCommonNameRule{CommonName: "cn.example.com"}},
						{DevicePosture: &networkingv1alpha2.AccessGroupDevicePostureRule{IntegrationUID: "posture-id"}},
						{GSuite: &networkingv1alpha2.AccessGroupGSuiteRule{IdentityProviderID: "gsuite-id"}},
						{GitHub: &networkingv1alpha2.AccessGroupGitHubRule{Name: "org", IdentityProviderID: "github-id"}},
						{Azure: &networkingv1alpha2.AccessGroupAzureRule{ID: "azure-id", IdentityProviderID: "azure-id"}},
						{Okta: &networkingv1alpha2.AccessGroupOktaRule{Name: "okta-group", IdentityProviderID: "okta-id"}},
						{OIDC: &networkingv1alpha2.AccessGroupOIDCRule{ClaimName: "claim", ClaimValue: "value", IdentityProviderID: "oidc-id"}},
						{SAML: &networkingv1alpha2.AccessGroupSAMLRule{AttributeName: "attr", AttributeValue: "val", IdentityProviderID: "saml-id"}},
						{AuthMethod: &networkingv1alpha2.AccessGroupAuthMethodRule{AuthMethod: "mfa"}},
						{AuthContext: &networkingv1alpha2.AccessGroupAuthContextRule{
							ID:                 "auth-ctx-id",
							AcID:               "ac-id",
							IdentityProviderID: "azure-id",
						}},
						{LoginMethod: &networkingv1alpha2.AccessGroupLoginMethodRule{ID: "login-method-id"}},
						{ExternalEvaluation: &networkingv1alpha2.AccessGroupExternalEvaluationRule{EvaluateURL: "https://eval.example.com"}},
					},
				},
			},
		},
	}

	r := setupReconciler(t)
	r.app = app

	policies, err := r.resolvePolicies()

	assert.NoError(t, err)
	assert.Len(t, policies, 1)
	assert.Len(t, policies[0].Include, 23, "Should have all 23 rule types")

	// Verify specific rule types
	rules := policies[0].Include
	assert.NotNil(t, rules[0].Email)
	assert.NotNil(t, rules[1].EmailDomain)
	assert.NotNil(t, rules[2].EmailList)
	assert.True(t, rules[3].Everyone)
	assert.NotNil(t, rules[4].IPRanges)
	assert.NotNil(t, rules[5].IPList)
	assert.NotNil(t, rules[6].Country)
	assert.NotNil(t, rules[7].Group)
	assert.NotNil(t, rules[8].ServiceToken)
	assert.True(t, rules[9].AnyValidServiceToken)
	assert.True(t, rules[10].Certificate)
	assert.NotNil(t, rules[11].CommonName)
	assert.NotNil(t, rules[12].DevicePosture)
	assert.NotNil(t, rules[13].GSuite)
	assert.NotNil(t, rules[14].GitHub)
	assert.NotNil(t, rules[15].Azure)
	assert.NotNil(t, rules[16].Okta)
	assert.NotNil(t, rules[17].OIDC)
	assert.NotNil(t, rules[18].SAML)
	assert.NotNil(t, rules[19].AuthMethod)
	assert.NotNil(t, rules[20].AuthContext)
	assert.NotNil(t, rules[21].LoginMethod)
	assert.NotNil(t, rules[22].ExternalEvaluation)
}

// TestAccessPolicyConfigMethods tests the HasInlineRules and HasGroupReference methods.
func TestAccessPolicyConfigMethods(t *testing.T) {
	tests := []struct {
		name            string
		config          accesssvc.AccessPolicyConfig
		wantHasInline   bool
		wantHasGroupRef bool
	}{
		{
			name:            "empty config",
			config:          accesssvc.AccessPolicyConfig{},
			wantHasInline:   false,
			wantHasGroupRef: false,
		},
		{
			name: "config with GroupID",
			config: accesssvc.AccessPolicyConfig{
				GroupID: "group-123",
			},
			wantHasInline:   false,
			wantHasGroupRef: true,
		},
		{
			name: "config with CloudflareGroupID",
			config: accesssvc.AccessPolicyConfig{
				CloudflareGroupID: "cf-group-123",
			},
			wantHasInline:   false,
			wantHasGroupRef: true,
		},
		{
			name: "config with CloudflareGroupName",
			config: accesssvc.AccessPolicyConfig{
				CloudflareGroupName: "My Group",
			},
			wantHasInline:   false,
			wantHasGroupRef: true,
		},
		{
			name: "config with K8sAccessGroupName",
			config: accesssvc.AccessPolicyConfig{
				K8sAccessGroupName: "k8s-group",
			},
			wantHasInline:   false,
			wantHasGroupRef: true,
		},
		{
			name: "config with Include rules",
			config: accesssvc.AccessPolicyConfig{
				Include: []networkingv1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			wantHasInline:   true,
			wantHasGroupRef: false,
		},
		{
			name: "config with Exclude rules only",
			config: accesssvc.AccessPolicyConfig{
				Exclude: []networkingv1alpha2.AccessGroupRule{
					{Email: &networkingv1alpha2.AccessGroupEmailRule{Email: "blocked@example.com"}},
				},
			},
			wantHasInline:   true,
			wantHasGroupRef: false,
		},
		{
			name: "config with Require rules only",
			config: accesssvc.AccessPolicyConfig{
				Require: []networkingv1alpha2.AccessGroupRule{
					{Certificate: true},
				},
			},
			wantHasInline:   true,
			wantHasGroupRef: false,
		},
		{
			name: "config with both inline and group ref",
			config: accesssvc.AccessPolicyConfig{
				GroupID: "group-123",
				Include: []networkingv1alpha2.AccessGroupRule{
					{Everyone: true},
				},
			},
			wantHasInline:   true,
			wantHasGroupRef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantHasInline, tt.config.HasInlineRules(), "HasInlineRules mismatch")
			assert.Equal(t, tt.wantHasGroupRef, tt.config.HasGroupReference(), "HasGroupReference mismatch")
		})
	}
}
