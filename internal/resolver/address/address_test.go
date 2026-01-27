// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package address

import (
	"testing"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

//nolint:revive // cognitive complexity acceptable for table-driven test
func TestParseAddress(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		wantIPv4  bool
		wantIPv6  bool
		wantHost  bool
		wantValue string
	}{
		{
			name:      "IPv4 address",
			addr:      "192.168.1.1",
			wantIPv4:  true,
			wantValue: "192.168.1.1",
		},
		{
			name:      "IPv6 address",
			addr:      "2001:db8::1",
			wantIPv6:  true,
			wantValue: "2001:db8::1",
		},
		{
			name:      "hostname",
			addr:      "example.com",
			wantHost:  true,
			wantValue: "example.com",
		},
		{
			name:      "LoadBalancer hostname",
			addr:      "a1234567890.elb.us-west-2.amazonaws.com",
			wantHost:  true,
			wantValue: "a1234567890.elb.us-west-2.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAddress(tt.addr)
			if got.Value != tt.wantValue {
				t.Errorf("ParseAddress() value = %v, want %v", got.Value, tt.wantValue)
			}
			if got.IsIPv4 != tt.wantIPv4 {
				t.Errorf("ParseAddress() IsIPv4 = %v, want %v", got.IsIPv4, tt.wantIPv4)
			}
			if got.IsIPv6 != tt.wantIPv6 {
				t.Errorf("ParseAddress() IsIPv6 = %v, want %v", got.IsIPv6, tt.wantIPv6)
			}
			if got.IsHostname != tt.wantHost {
				t.Errorf("ParseAddress() IsHostname = %v, want %v", got.IsHostname, tt.wantHost)
			}
		})
	}
}

func TestDetermineRecordType(t *testing.T) {
	tests := []struct {
		name string
		addr ResolvedAddress
		want string
	}{
		{
			name: "IPv4 returns A",
			addr: ResolvedAddress{Value: "1.2.3.4", IsIPv4: true},
			want: "A",
		},
		{
			name: "IPv6 returns AAAA",
			addr: ResolvedAddress{Value: "2001:db8::1", IsIPv6: true},
			want: "AAAA",
		},
		{
			name: "hostname returns CNAME",
			addr: ResolvedAddress{Value: "example.com", IsHostname: true},
			want: "CNAME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetermineRecordType(tt.addr); got != tt.want {
				t.Errorf("DetermineRecordType() = %v, want %v", got, tt.want)
			}
		})
	}
}

//nolint:revive // cognitive complexity acceptable for table-driven test
func TestSelectAddresses(t *testing.T) {
	addresses := []ResolvedAddress{
		{Value: "1.1.1.1", IsIPv4: true},
		{Value: "2001:db8::1", IsIPv6: true},
		{Value: "2.2.2.2", IsIPv4: true},
	}

	tests := []struct {
		name      string
		addresses []ResolvedAddress
		policy    v1alpha2.AddressSelectionPolicy
		wantLen   int
		wantFirst string
	}{
		{
			name:      "First policy",
			addresses: addresses,
			policy:    v1alpha2.AddressSelectionFirst,
			wantLen:   1,
			wantFirst: "1.1.1.1",
		},
		{
			name:      "All policy",
			addresses: addresses,
			policy:    v1alpha2.AddressSelectionAll,
			wantLen:   3,
			wantFirst: "1.1.1.1",
		},
		{
			name:      "PreferIPv4 policy",
			addresses: addresses,
			policy:    v1alpha2.AddressSelectionPreferIPv4,
			wantLen:   1,
			wantFirst: "1.1.1.1",
		},
		{
			name:      "PreferIPv6 policy",
			addresses: addresses,
			policy:    v1alpha2.AddressSelectionPreferIPv6,
			wantLen:   1,
			wantFirst: "2001:db8::1",
		},
		{
			name:      "Empty policy defaults to First",
			addresses: addresses,
			policy:    "",
			wantLen:   1,
			wantFirst: "1.1.1.1",
		},
		{
			name:      "Empty addresses",
			addresses: []ResolvedAddress{},
			policy:    v1alpha2.AddressSelectionFirst,
			wantLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectAddresses(tt.addresses, tt.policy)
			if len(got) != tt.wantLen {
				t.Errorf("SelectAddresses() len = %v, want %v", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Value != tt.wantFirst {
				t.Errorf("SelectAddresses() first = %v, want %v", got[0].Value, tt.wantFirst)
			}
		})
	}
}

func TestAddressesToStrings(t *testing.T) {
	addresses := []ResolvedAddress{
		{Value: "1.1.1.1", IsIPv4: true},
		{Value: "example.com", IsHostname: true},
	}

	got := AddressesToStrings(addresses)
	if len(got) != 2 {
		t.Fatalf("AddressesToStrings() len = %v, want 2", len(got))
	}
	if got[0] != "1.1.1.1" {
		t.Errorf("AddressesToStrings()[0] = %v, want 1.1.1.1", got[0])
	}
	if got[1] != "example.com" {
		t.Errorf("AddressesToStrings()[1] = %v, want example.com", got[1])
	}
}

func TestDNSRecordSourceRef_CountSources(t *testing.T) {
	tests := []struct {
		name      string
		sourceRef *v1alpha2.DNSRecordSourceRef
		want      int
	}{
		{
			name:      "nil sourceRef",
			sourceRef: nil,
			want:      0,
		},
		{
			name:      "empty sourceRef",
			sourceRef: &v1alpha2.DNSRecordSourceRef{},
			want:      0,
		},
		{
			name: "service only",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Service: &v1alpha2.ServiceDNSSource{Name: "test"},
			},
			want: 1,
		},
		{
			name: "multiple sources",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Service: &v1alpha2.ServiceDNSSource{Name: "test"},
				Ingress: &v1alpha2.IngressDNSSource{Name: "test"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sourceRef.CountSources(); got != tt.want {
				t.Errorf("CountSources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDNSRecordSourceRef_GetSourceType(t *testing.T) {
	tests := []struct {
		name      string
		sourceRef *v1alpha2.DNSRecordSourceRef
		want      string
	}{
		{
			name:      "nil sourceRef",
			sourceRef: nil,
			want:      "",
		},
		{
			name: "service",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Service: &v1alpha2.ServiceDNSSource{Name: "test"},
			},
			want: "Service",
		},
		{
			name: "ingress",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Ingress: &v1alpha2.IngressDNSSource{Name: "test"},
			},
			want: "Ingress",
		},
		{
			name: "httpRoute",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				HTTPRoute: &v1alpha2.HTTPRouteDNSSource{Name: "test"},
			},
			want: "HTTPRoute",
		},
		{
			name: "gateway",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Gateway: &v1alpha2.GatewayDNSSource{Name: "test"},
			},
			want: "Gateway",
		},
		{
			name: "node",
			sourceRef: &v1alpha2.DNSRecordSourceRef{
				Node: &v1alpha2.NodeDNSSource{Name: "test"},
			},
			want: "Node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sourceRef.GetSourceType(); got != tt.want {
				t.Errorf("GetSourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}
