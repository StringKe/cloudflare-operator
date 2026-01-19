// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

//go:build e2e

package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/test/e2e/framework"
)

// TestAccessGroupLifecycle tests the complete lifecycle of an AccessGroup resource
func TestAccessGroupLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	var groupID string

	t.Run("CreateAccessGroup", func(t *testing.T) {
		group := &v1alpha2.AccessGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-access-group",
			},
			Spec: v1alpha2.AccessGroupSpec{
				Name: "E2E Test Access Group",
				Include: []v1alpha2.AccessGroupRule{
					{
						Email: &v1alpha2.AccessGroupEmailRule{
							Email: "test@example.com",
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, group)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(group, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessGroup should become ready")

		// Verify status
		var fetched v1alpha2.AccessGroup
		err = f.Client.Get(ctx, types.NamespacedName{Name: group.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.GroupID, "GroupID should be set")
		groupID = fetched.Status.GroupID
	})

	t.Run("UpdateAccessGroup", func(t *testing.T) {
		var group v1alpha2.AccessGroup
		err := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-access-group"}, &group)
		require.NoError(t, err)

		// Add another rule
		group.Spec.Include = append(group.Spec.Include, v1alpha2.AccessGroupRule{
			EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
				Domain: "example.org",
			},
		})
		err = f.Client.Update(ctx, &group)
		require.NoError(t, err)

		// Wait for reconciliation
		err = f.WaitForCondition(&group, "Ready", metav1.ConditionTrue, 30*time.Second)
		require.NoError(t, err)

		// Verify GroupID is preserved
		var fetched v1alpha2.AccessGroup
		err = f.Client.Get(ctx, types.NamespacedName{Name: group.Name}, &fetched)
		require.NoError(t, err)
		assert.Equal(t, groupID, fetched.Status.GroupID, "GroupID should be preserved after update")
	})

	t.Run("DeleteAccessGroup", func(t *testing.T) {
		var group v1alpha2.AccessGroup
		err := f.Client.Get(ctx, types.NamespacedName{Name: "e2e-test-access-group"}, &group)
		require.NoError(t, err)

		err = f.Client.Delete(ctx, &group)
		require.NoError(t, err)

		err = f.WaitForDeletion(&group, 2*time.Minute)
		assert.NoError(t, err, "AccessGroup should be deleted")
	})
}

// TestAccessIdentityProviderLifecycle tests the AccessIdentityProvider resource
func TestAccessIdentityProviderLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	t.Run("CreateOTPIdentityProvider", func(t *testing.T) {
		idp := &v1alpha2.AccessIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-otp-idp",
			},
			Spec: v1alpha2.AccessIdentityProviderSpec{
				Name: "E2E OTP Provider",
				Type: "onetimepin",
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, idp)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(idp, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessIdentityProvider should become ready")

		// Verify status
		var fetched v1alpha2.AccessIdentityProvider
		err = f.Client.Get(ctx, types.NamespacedName{Name: idp.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ProviderID, "ProviderID should be set")

		// Cleanup
		defer func() {
			_ = f.Client.Delete(ctx, idp)
			_ = f.WaitForDeletion(idp, time.Minute)
		}()
	})
}

// TestAccessApplicationLifecycle tests the AccessApplication resource
func TestAccessApplicationLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	// Create a prerequisite AccessGroup
	group := &v1alpha2.AccessGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-app-access-group",
		},
		Spec: v1alpha2.AccessGroupSpec{
			Name: "App Access Group",
			Include: []v1alpha2.AccessGroupRule{
				{
					Everyone: true,
				},
			},
			Cloudflare: v1alpha2.CloudflareDetails{
				CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
					Name: testCredentialsName,
				},
			},
		},
	}

	err = f.Client.Create(ctx, group)
	require.NoError(t, err)
	defer func() {
		_ = f.Client.Delete(ctx, group)
		_ = f.WaitForDeletion(group, time.Minute)
	}()

	err = f.WaitForCondition(group, "Ready", metav1.ConditionTrue, 2*time.Minute)
	require.NoError(t, err, "AccessGroup must be ready before creating AccessApplication")

	t.Run("CreateSelfHostedApplication", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Test Application",
				Domain:          "e2e-test.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication should become ready")

		// Verify status
		var fetched v1alpha2.AccessApplication
		err = f.Client.Get(ctx, types.NamespacedName{Name: app.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ApplicationID, "ApplicationID should be set")

		// Cleanup
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()
	})
}

// TestAccessApplicationWithInlinePolicies tests AccessApplication with inline policy rules
// This tests the new inline rules mode where include/exclude/require rules are specified directly
// instead of referencing an AccessGroup.
func TestAccessApplicationWithInlinePolicies(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	t.Run("CreateApplicationWithInlineEmailPolicy", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-inline-email-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Inline Email Policy App",
				Domain:          "e2e-inline-email.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Policies: []v1alpha2.AccessPolicyRef{
					{
						Decision:   "allow",
						Precedence: 1,
						PolicyName: "Email Allow Policy",
						Include: []v1alpha2.AccessGroupRule{
							{
								Email: &v1alpha2.AccessGroupEmailRule{
									Email: "admin@example.com",
								},
							},
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication with inline email policy should become ready")

		// Verify status
		var fetched v1alpha2.AccessApplication
		err = f.Client.Get(ctx, types.NamespacedName{Name: app.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ApplicationID, "ApplicationID should be set")
	})

	t.Run("CreateApplicationWithInlineEmailDomainPolicy", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-inline-domain-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Inline Domain Policy App",
				Domain:          "e2e-inline-domain.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Policies: []v1alpha2.AccessPolicyRef{
					{
						Decision:   "allow",
						Precedence: 1,
						PolicyName: "Domain Allow Policy",
						Include: []v1alpha2.AccessGroupRule{
							{
								EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
									Domain: "example.com",
								},
							},
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication with inline domain policy should become ready")
	})

	t.Run("CreateApplicationWithMultipleInlinePolicies", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-inline-multi-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Multiple Inline Policies App",
				Domain:          "e2e-inline-multi.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Policies: []v1alpha2.AccessPolicyRef{
					{
						Decision:   "allow",
						Precedence: 1,
						PolicyName: "Admin Allow Policy",
						Include: []v1alpha2.AccessGroupRule{
							{
								Email: &v1alpha2.AccessGroupEmailRule{
									Email: "admin@example.com",
								},
							},
						},
					},
					{
						Decision:   "allow",
						Precedence: 2,
						PolicyName: "Domain Allow Policy",
						Include: []v1alpha2.AccessGroupRule{
							{
								EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
									Domain: "example.com",
								},
							},
						},
						Exclude: []v1alpha2.AccessGroupRule{
							{
								Email: &v1alpha2.AccessGroupEmailRule{
									Email: "blocked@example.com",
								},
							},
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication with multiple inline policies should become ready")

		// Verify status
		var fetched v1alpha2.AccessApplication
		err = f.Client.Get(ctx, types.NamespacedName{Name: app.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.ApplicationID, "ApplicationID should be set")
	})

	t.Run("CreateApplicationWithEveryonePolicy", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-inline-everyone-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Everyone Policy App",
				Domain:          "e2e-inline-everyone.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Policies: []v1alpha2.AccessPolicyRef{
					{
						Decision:   "allow",
						Precedence: 1,
						PolicyName: "Everyone Allow Policy",
						Include: []v1alpha2.AccessGroupRule{
							{
								Everyone: true,
							},
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication with everyone policy should become ready")
	})

	t.Run("CreateApplicationWithRequirePolicy", func(t *testing.T) {
		app := &v1alpha2.AccessApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-inline-require-app",
			},
			Spec: v1alpha2.AccessApplicationSpec{
				Name:            "E2E Require Policy App",
				Domain:          "e2e-inline-require.example.com",
				Type:            "self_hosted",
				SessionDuration: "24h",
				Policies: []v1alpha2.AccessPolicyRef{
					{
						Decision:   "allow",
						Precedence: 1,
						PolicyName: "Domain with MFA Requirement",
						Include: []v1alpha2.AccessGroupRule{
							{
								EmailDomain: &v1alpha2.AccessGroupEmailDomainRule{
									Domain: "example.com",
								},
							},
						},
						Require: []v1alpha2.AccessGroupRule{
							{
								AuthMethod: &v1alpha2.AccessGroupAuthMethodRule{
									AuthMethod: "mfa",
								},
							},
						},
					},
				},
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, app)
		require.NoError(t, err)
		defer func() {
			_ = f.Client.Delete(ctx, app)
			_ = f.WaitForDeletion(app, time.Minute)
		}()

		// Wait for Ready condition
		err = f.WaitForCondition(app, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessApplication with require policy should become ready")
	})
}

// TestAccessServiceTokenLifecycle tests the AccessServiceToken resource
func TestAccessServiceTokenLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	opts := framework.DefaultOptions()
	opts.UseExistingCluster = true
	f, err := framework.New(opts)
	require.NoError(t, err)
	defer f.Cleanup()

	ctx := f.Context()

	// Ensure operator namespace exists and create credentials
	require.NoError(t, f.EnsureNamespaceExists(framework.OperatorNamespace))
	require.NoError(t, f.CreateCloudflareCredentials(testCredentialsName, testAPIToken, testAccountID, true))

	t.Run("CreateServiceToken", func(t *testing.T) {
		token := &v1alpha2.AccessServiceToken{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-test-service-token",
			},
			Spec: v1alpha2.AccessServiceTokenSpec{
				Name: "E2E Test Service Token",
				Cloudflare: v1alpha2.CloudflareDetails{
					CredentialsRef: &v1alpha2.CloudflareCredentialsRef{
						Name: testCredentialsName,
					},
				},
			},
		}

		err := f.Client.Create(ctx, token)
		require.NoError(t, err)

		// Wait for Ready condition
		err = f.WaitForCondition(token, "Ready", metav1.ConditionTrue, 2*time.Minute)
		assert.NoError(t, err, "AccessServiceToken should become ready")

		// Verify status
		var fetched v1alpha2.AccessServiceToken
		err = f.Client.Get(ctx, types.NamespacedName{Name: token.Name}, &fetched)
		require.NoError(t, err)
		assert.NotEmpty(t, fetched.Status.TokenID, "TokenID should be set")

		// Cleanup
		defer func() {
			_ = f.Client.Delete(ctx, token)
			_ = f.WaitForDeletion(token, time.Minute)
		}()
	})
}
