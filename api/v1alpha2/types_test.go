// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTunnelTypes(t *testing.T) {
	t.Run("ExistingTunnel with ID", func(t *testing.T) {
		et := ExistingTunnel{
			Id:   "tunnel-123",
			Name: "my-tunnel",
		}
		assert.Equal(t, "tunnel-123", et.Id)
		assert.Equal(t, "my-tunnel", et.Name)
	})

	t.Run("NewTunnel", func(t *testing.T) {
		nt := NewTunnel{Name: "new-tunnel"}
		assert.Equal(t, "new-tunnel", nt.Name)
	})

	t.Run("CloudflareCredentialsRef", func(t *testing.T) {
		ref := CloudflareCredentialsRef{Name: "my-credentials"}
		assert.Equal(t, "my-credentials", ref.Name)
	})

	t.Run("CloudflareDetails full", func(t *testing.T) {
		details := CloudflareDetails{
			CredentialsRef: &CloudflareCredentialsRef{Name: "creds"},
			Domain:         "example.com",
			ZoneId:         "zone-123",
			Secret:         "secret",
			AccountName:    "account",
			AccountId:      "acc-123",
			Email:          "user@example.com",
		}
		require.NotNil(t, details.CredentialsRef)
		assert.Equal(t, "creds", details.CredentialsRef.Name)
		assert.Equal(t, "example.com", details.Domain)
	})
}

func TestTunnelSpec(t *testing.T) {
	spec := TunnelSpec{
		DeployPatch:    "{}",
		NoTlsVerify:    true,
		OriginCaPool:   "ca-pool",
		Protocol:       "http2",
		FallbackTarget: "http_status:404",
		Cloudflare: CloudflareDetails{
			Domain: "example.com",
		},
		NewTunnel: &NewTunnel{Name: "test-tunnel"},
	}

	assert.True(t, spec.NoTlsVerify)
	assert.Equal(t, "ca-pool", spec.OriginCaPool)
	assert.Equal(t, "http2", spec.Protocol)
	assert.Equal(t, "http_status:404", spec.FallbackTarget)
	require.NotNil(t, spec.NewTunnel)
	assert.Equal(t, "test-tunnel", spec.NewTunnel.Name)
}

func TestTunnelStatus(t *testing.T) {
	status := TunnelStatus{
		TunnelId:           "tunnel-123",
		TunnelName:         "my-tunnel",
		AccountId:          "acc-123",
		ZoneId:             "zone-123",
		ObservedGeneration: 5,
	}

	assert.Equal(t, "tunnel-123", status.TunnelId)
	assert.Equal(t, "my-tunnel", status.TunnelName)
	assert.Equal(t, "acc-123", status.AccountId)
	assert.Equal(t, int64(5), status.ObservedGeneration)
}

func TestDNSRecordTypes(t *testing.T) {
	t.Run("DNSRecordSpec", func(t *testing.T) {
		priority := 10
		spec := DNSRecordSpec{
			Name:     "www.example.com",
			Type:     "A",
			Content:  "1.2.3.4",
			TTL:      300,
			Proxied:  true,
			Priority: &priority,
			Comment:  "Main web server",
			Tags:     []string{"production", "web"},
		}

		assert.Equal(t, "www.example.com", spec.Name)
		assert.Equal(t, "A", spec.Type)
		assert.Equal(t, "1.2.3.4", spec.Content)
		assert.Equal(t, 300, spec.TTL)
		assert.True(t, spec.Proxied)
		require.NotNil(t, spec.Priority)
		assert.Equal(t, 10, *spec.Priority)
		assert.Len(t, spec.Tags, 2)
	})

	t.Run("DNSRecordData SRV", func(t *testing.T) {
		data := DNSRecordData{
			Service: "_http",
			Proto:   "_tcp",
			Weight:  10,
			Port:    80,
			Target:  "target.example.com",
		}

		assert.Equal(t, "_http", data.Service)
		assert.Equal(t, "_tcp", data.Proto)
		assert.Equal(t, 10, data.Weight)
		assert.Equal(t, 80, data.Port)
	})

	t.Run("DNSRecordData CAA", func(t *testing.T) {
		data := DNSRecordData{
			Flags: 0,
			Tag:   "issue",
			Value: "letsencrypt.org",
		}

		assert.Equal(t, 0, data.Flags)
		assert.Equal(t, "issue", data.Tag)
		assert.Equal(t, "letsencrypt.org", data.Value)
	})
}

func TestVirtualNetworkTypes(t *testing.T) {
	t.Run("VirtualNetworkSpec", func(t *testing.T) {
		spec := VirtualNetworkSpec{
			Comment:          "Test VNet",
			IsDefaultNetwork: true,
			Cloudflare: CloudflareDetails{
				CredentialsRef: &CloudflareCredentialsRef{Name: "creds"},
			},
		}

		assert.Equal(t, "Test VNet", spec.Comment)
		assert.True(t, spec.IsDefaultNetwork)
		require.NotNil(t, spec.Cloudflare.CredentialsRef)
	})

	t.Run("VirtualNetworkStatus", func(t *testing.T) {
		status := VirtualNetworkStatus{
			VirtualNetworkId:   "vnet-123",
			AccountId:          "acc-123",
			State:              "active",
			ObservedGeneration: 3,
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		}

		assert.Equal(t, "vnet-123", status.VirtualNetworkId)
		assert.Equal(t, "active", status.State)
		assert.Len(t, status.Conditions, 1)
	})
}

func TestNetworkRouteTypes(t *testing.T) {
	t.Run("NetworkRouteSpec", func(t *testing.T) {
		spec := NetworkRouteSpec{
			Network: "10.0.0.0/8",
			Comment: "Private network",
			TunnelRef: TunnelRef{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			VirtualNetworkRef: &VirtualNetworkRef{
				Name: "my-vnet",
			},
		}

		assert.Equal(t, "10.0.0.0/8", spec.Network)
		assert.Equal(t, "Private network", spec.Comment)
		assert.Equal(t, "my-tunnel", spec.TunnelRef.Name)
		require.NotNil(t, spec.VirtualNetworkRef)
		assert.Equal(t, "my-vnet", spec.VirtualNetworkRef.Name)
	})

	t.Run("NetworkRouteStatus", func(t *testing.T) {
		status := NetworkRouteStatus{
			Network:            "10.0.0.0/8",
			TunnelID:           "tunnel-456",
			VirtualNetworkID:   "vnet-789",
			State:              "active",
			ObservedGeneration: 2,
		}

		assert.Equal(t, "10.0.0.0/8", status.Network)
		assert.Equal(t, "tunnel-456", status.TunnelID)
		assert.Equal(t, "vnet-789", status.VirtualNetworkID)
	})
}

func TestAccessApplicationTypes(t *testing.T) {
	t.Run("AccessApplicationSpec basic", func(t *testing.T) {
		spec := AccessApplicationSpec{
			Name:                   "My App",
			Domain:                 "app.example.com",
			Type:                   "self_hosted",
			SessionDuration:        "24h",
			AllowedIdps:            []string{"idp-1", "idp-2"},
			AutoRedirectToIdentity: true,
		}

		assert.Equal(t, "My App", spec.Name)
		assert.Equal(t, "app.example.com", spec.Domain)
		assert.Equal(t, "self_hosted", spec.Type)
		assert.True(t, spec.AutoRedirectToIdentity)
		assert.Len(t, spec.AllowedIdps, 2)
	})

	t.Run("AccessApplicationSpec with policies", func(t *testing.T) {
		spec := AccessApplicationSpec{
			Name:   "App with policies",
			Domain: "app.example.com",
			Policies: []AccessPolicyRef{
				{
					Name:       "admin-group",
					Decision:   "allow",
					Precedence: 1,
				},
			},
		}

		require.Len(t, spec.Policies, 1)
		assert.Equal(t, "admin-group", spec.Policies[0].Name)
		assert.Equal(t, "allow", spec.Policies[0].Decision)
	})
}

func TestAccessGroupTypes(t *testing.T) {
	t.Run("AccessGroupRule email", func(t *testing.T) {
		rule := AccessGroupRule{
			Email: &AccessGroupEmailRule{Email: "user@example.com"},
		}
		require.NotNil(t, rule.Email)
		assert.Equal(t, "user@example.com", rule.Email.Email)
	})

	t.Run("AccessGroupRule email domain", func(t *testing.T) {
		rule := AccessGroupRule{
			EmailDomain: &AccessGroupEmailDomainRule{Domain: "example.com"},
		}
		require.NotNil(t, rule.EmailDomain)
		assert.Equal(t, "example.com", rule.EmailDomain.Domain)
	})

	t.Run("AccessGroupRule IPRanges", func(t *testing.T) {
		rule := AccessGroupRule{
			IPRanges: &AccessGroupIPRangesRule{IP: []string{"10.0.0.0/8"}},
		}
		require.NotNil(t, rule.IPRanges)
		assert.Len(t, rule.IPRanges.IP, 1)
		assert.Equal(t, "10.0.0.0/8", rule.IPRanges.IP[0])
	})

	t.Run("AccessGroupRule everyone", func(t *testing.T) {
		rule := AccessGroupRule{
			Everyone: true,
		}
		assert.True(t, rule.Everyone)
	})

	t.Run("AccessGroupRule service token", func(t *testing.T) {
		rule := AccessGroupRule{
			ServiceToken: &AccessGroupServiceTokenRule{TokenID: "token-123"},
		}
		require.NotNil(t, rule.ServiceToken)
		assert.Equal(t, "token-123", rule.ServiceToken.TokenID)
	})

	t.Run("AccessGroupRule certificate", func(t *testing.T) {
		rule := AccessGroupRule{
			Certificate: true,
		}
		assert.True(t, rule.Certificate)
	})

	t.Run("AccessGroupRule GitHub", func(t *testing.T) {
		rule := AccessGroupRule{
			GitHub: &AccessGroupGitHubRule{
				Name:   "my-org",
				Teams:  []string{"team-a", "team-b"},
				IdpRef: &AccessIdentityProviderRefV2{CloudflareID: "idp-github"},
			},
		}
		require.NotNil(t, rule.GitHub)
		assert.Equal(t, "my-org", rule.GitHub.Name)
		assert.Len(t, rule.GitHub.Teams, 2)
	})

	t.Run("AccessGroupRule Azure AD", func(t *testing.T) {
		rule := AccessGroupRule{
			Azure: &AccessGroupAzureRule{
				ID:     "azure-group-id",
				IdpRef: &AccessIdentityProviderRefV2{CloudflareID: "idp-azure"},
			},
		}
		require.NotNil(t, rule.Azure)
		assert.Equal(t, "azure-group-id", rule.Azure.ID)
	})

	t.Run("AccessGroupRule SAML", func(t *testing.T) {
		rule := AccessGroupRule{
			SAML: &AccessGroupSAMLRule{
				AttributeName:  "department",
				AttributeValue: "engineering",
				IdpRef:         &AccessIdentityProviderRefV2{CloudflareID: "idp-saml"},
			},
		}
		require.NotNil(t, rule.SAML)
		assert.Equal(t, "department", rule.SAML.AttributeName)
		assert.Equal(t, "engineering", rule.SAML.AttributeValue)
	})

	t.Run("AccessGroupSpec", func(t *testing.T) {
		isDefault := true
		spec := AccessGroupSpec{
			Name:      "Admin Group",
			IsDefault: &isDefault,
			Include: []AccessGroupRule{
				{EmailDomain: &AccessGroupEmailDomainRule{Domain: "admin.example.com"}},
			},
			Exclude: []AccessGroupRule{
				{Email: &AccessGroupEmailRule{Email: "guest@admin.example.com"}},
			},
			Require: []AccessGroupRule{
				{Certificate: true},
			},
		}

		assert.Equal(t, "Admin Group", spec.Name)
		assert.True(t, *spec.IsDefault)
		assert.Len(t, spec.Include, 1)
		assert.Len(t, spec.Exclude, 1)
		assert.Len(t, spec.Require, 1)
	})
}

func TestPrivateServiceTypes(t *testing.T) {
	t.Run("PrivateServiceSpec", func(t *testing.T) {
		spec := PrivateServiceSpec{
			ServiceRef: ServiceRef{
				Name: "my-service",
				Port: 8080,
			},
			TunnelRef: TunnelRef{
				Kind: "ClusterTunnel",
				Name: "my-tunnel",
			},
			VirtualNetworkRef: &VirtualNetworkRef{
				Name: "my-vnet",
			},
			Protocol: "tcp",
		}

		assert.Equal(t, "my-service", spec.ServiceRef.Name)
		assert.Equal(t, int32(8080), spec.ServiceRef.Port)
		assert.Equal(t, "my-tunnel", spec.TunnelRef.Name)
		require.NotNil(t, spec.VirtualNetworkRef)
		assert.Equal(t, "tcp", spec.Protocol)
	})

	t.Run("PrivateServiceStatus", func(t *testing.T) {
		status := PrivateServiceStatus{
			State:              "active",
			Network:            "10.0.0.0/32",
			ObservedGeneration: 2,
		}

		assert.Equal(t, "active", status.State)
		assert.Equal(t, "10.0.0.0/32", status.Network)
	})
}

func TestR2BucketTypes(t *testing.T) {
	t.Run("R2BucketSpec", func(t *testing.T) {
		spec := R2BucketSpec{
			Name:           "my-bucket",
			LocationHint:   R2LocationENAM,
			DeletionPolicy: "Delete",
		}

		assert.Equal(t, "my-bucket", spec.Name)
		assert.Equal(t, R2LocationENAM, spec.LocationHint)
		assert.Equal(t, "Delete", spec.DeletionPolicy)
	})

	t.Run("R2BucketStatus", func(t *testing.T) {
		status := R2BucketStatus{
			BucketName:         "my-bucket",
			Location:           "ENAM",
			State:              R2BucketStateReady,
			ObservedGeneration: 1,
		}

		assert.Equal(t, "my-bucket", status.BucketName)
		assert.Equal(t, "ENAM", status.Location)
		assert.Equal(t, R2BucketStateReady, status.State)
	})
}

func TestGatewayRuleTypes(t *testing.T) {
	t.Run("GatewayRuleSpec", func(t *testing.T) {
		blockPageEnabled := true
		spec := GatewayRuleSpec{
			Name:        "Block malware",
			Description: "Block known malware domains",
			Action:      "block",
			Enabled:     true,
			Precedence:  1,
			Filters:     []string{"dns"},
			Traffic:     "any(dns.domains[*] in $malware_domains)",
			RuleSettings: &GatewayRuleSettings{
				BlockPageEnabled: &blockPageEnabled,
				BlockReason:      "Malware detected",
			},
		}

		assert.Equal(t, "Block malware", spec.Name)
		assert.Equal(t, "block", spec.Action)
		assert.True(t, spec.Enabled)
		assert.Equal(t, 1, spec.Precedence)
		assert.Contains(t, spec.Traffic, "malware_domains")
	})
}

func TestZoneRulesetTypes(t *testing.T) {
	t.Run("ZoneRulesetSpec", func(t *testing.T) {
		spec := ZoneRulesetSpec{
			Zone:        "example.com",
			Phase:       RulesetPhaseHTTPRequestTransform,
			Description: "Transform rules",
			Rules: []RulesetRule{
				{
					Description: "Rewrite URL",
					Expression:  "http.request.uri.path contains \"/old\"",
					Action:      RulesetRuleActionRewrite,
					Enabled:     true,
				},
			},
		}

		assert.Equal(t, RulesetPhaseHTTPRequestTransform, spec.Phase)
		assert.Equal(t, "example.com", spec.Zone)
		require.Len(t, spec.Rules, 1)
		assert.Equal(t, RulesetRuleActionRewrite, spec.Rules[0].Action)
		assert.True(t, spec.Rules[0].Enabled)
	})
}

func TestCredentialsReferenceTypes(t *testing.T) {
	t.Run("CredentialsReference basic", func(t *testing.T) {
		ref := CredentialsReference{
			Name: "my-credentials",
		}
		assert.Equal(t, "my-credentials", ref.Name)
	})
}

func TestCloudflareCredentialsTypes(t *testing.T) {
	t.Run("CloudflareCredentialsSpec", func(t *testing.T) {
		spec := CloudflareCredentialsSpec{
			SecretRef: SecretReference{
				Name:      "cf-secret",
				Namespace: "cloudflare-system",
			},
			AccountID:     "acc-123",
			AccountName:   "My Account",
			DefaultDomain: "example.com",
		}

		assert.Equal(t, "cf-secret", spec.SecretRef.Name)
		assert.Equal(t, "cloudflare-system", spec.SecretRef.Namespace)
		assert.Equal(t, "acc-123", spec.AccountID)
		assert.Equal(t, "example.com", spec.DefaultDomain)
	})

	t.Run("CloudflareCredentialsStatus", func(t *testing.T) {
		status := CloudflareCredentialsStatus{
			State:              "active",
			AccountName:        "My Account",
			Validated:          true,
			ObservedGeneration: 1,
		}

		assert.Equal(t, "active", status.State)
		assert.Equal(t, "My Account", status.AccountName)
		assert.True(t, status.Validated)
	})
}

func TestCloudflareDeepCopy(t *testing.T) {
	t.Run("Tunnel DeepCopy", func(t *testing.T) {
		tunnel := &Tunnel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tunnel",
				Namespace: "default",
			},
			Spec: TunnelSpec{
				NoTlsVerify: true,
				Cloudflare: CloudflareDetails{
					Domain: "example.com",
				},
			},
		}

		copied := tunnel.DeepCopy()

		assert.Equal(t, tunnel.Name, copied.Name)
		assert.Equal(t, tunnel.Namespace, copied.Namespace)
		assert.Equal(t, tunnel.Spec.NoTlsVerify, copied.Spec.NoTlsVerify)
		assert.Equal(t, tunnel.Spec.Cloudflare.Domain, copied.Spec.Cloudflare.Domain)

		// Modify copy and verify original unchanged
		copied.Name = "modified"
		assert.NotEqual(t, tunnel.Name, copied.Name)
	})

	t.Run("VirtualNetwork DeepCopy", func(t *testing.T) {
		vnet := &VirtualNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-vnet",
			},
			Spec: VirtualNetworkSpec{
				Comment:          "Test",
				IsDefaultNetwork: true,
			},
		}

		copied := vnet.DeepCopy()

		assert.Equal(t, vnet.Name, copied.Name)
		assert.Equal(t, vnet.Spec.Comment, copied.Spec.Comment)
		assert.Equal(t, vnet.Spec.IsDefaultNetwork, copied.Spec.IsDefaultNetwork)
	})

	t.Run("DNSRecord DeepCopy", func(t *testing.T) {
		priority := 10
		record := &DNSRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-record",
				Namespace: "default",
			},
			Spec: DNSRecordSpec{
				Name:     "www.example.com",
				Type:     "A",
				Content:  "1.2.3.4",
				Priority: &priority,
			},
		}

		copied := record.DeepCopy()

		assert.Equal(t, record.Name, copied.Name)
		assert.Equal(t, record.Spec.Name, copied.Spec.Name)
		require.NotNil(t, copied.Spec.Priority)
		assert.Equal(t, *record.Spec.Priority, *copied.Spec.Priority)

		// Modify copy and verify original unchanged
		*copied.Spec.Priority = 20
		assert.NotEqual(t, *record.Spec.Priority, *copied.Spec.Priority)
	})

	t.Run("AccessGroup DeepCopy", func(t *testing.T) {
		group := &AccessGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-group",
			},
			Spec: AccessGroupSpec{
				Name: "Test Group",
				Include: []AccessGroupRule{
					{Email: &AccessGroupEmailRule{Email: "test@example.com"}},
				},
			},
		}

		copied := group.DeepCopy()

		assert.Equal(t, group.Name, copied.Name)
		assert.Equal(t, group.Spec.Name, copied.Spec.Name)
		require.Len(t, copied.Spec.Include, 1)
		assert.Equal(t, group.Spec.Include[0].Email.Email, copied.Spec.Include[0].Email.Email)
	})
}

func TestAccessGroupGetAccessGroupName(t *testing.T) {
	t.Run("returns spec name if set", func(t *testing.T) {
		group := &AccessGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k8s-name",
			},
			Spec: AccessGroupSpec{
				Name: "cloudflare-name",
			},
		}
		assert.Equal(t, "cloudflare-name", group.GetAccessGroupName())
	})

	t.Run("returns kubernetes name if spec name not set", func(t *testing.T) {
		group := &AccessGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k8s-name",
			},
			Spec: AccessGroupSpec{},
		}
		assert.Equal(t, "k8s-name", group.GetAccessGroupName())
	})
}
