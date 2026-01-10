/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package route

import (
	"fmt"
	"time"

	networkingv1alpha2 "github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/internal/clients/cf"
)

// OriginRequestBuilder helps build OriginRequestConfig from various sources.
type OriginRequestBuilder struct {
	config cf.OriginRequestConfig
}

// NewOriginRequestBuilder creates a new OriginRequestBuilder.
func NewOriginRequestBuilder() *OriginRequestBuilder {
	return &OriginRequestBuilder{
		config: cf.OriginRequestConfig{},
	}
}

// WithDefaults applies defaults from OriginRequestSpec.
// nolint:revive // Cognitive complexity is expected for mapping all fields
func (b *OriginRequestBuilder) WithDefaults(defaults *networkingv1alpha2.OriginRequestSpec) *OriginRequestBuilder {
	if defaults == nil {
		return b
	}

	b.config.NoTLSVerify = &defaults.NoTLSVerify
	b.config.Http2Origin = &defaults.HTTP2Origin

	if defaults.CAPool != "" {
		caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", defaults.CAPool)
		b.config.CAPool = &caPath
	}

	if defaults.ConnectTimeout != "" {
		if d, err := time.ParseDuration(defaults.ConnectTimeout); err == nil {
			b.config.ConnectTimeout = &d
		}
	}

	if defaults.TLSTimeout != "" {
		if d, err := time.ParseDuration(defaults.TLSTimeout); err == nil {
			b.config.TLSTimeout = &d
		}
	}

	if defaults.KeepAliveTimeout != "" {
		if d, err := time.ParseDuration(defaults.KeepAliveTimeout); err == nil {
			b.config.KeepAliveTimeout = &d
		}
	}

	if defaults.KeepAliveConnections != nil {
		b.config.KeepAliveConnections = defaults.KeepAliveConnections
	}

	if defaults.OriginServerName != "" {
		b.config.OriginServerName = &defaults.OriginServerName
	}

	if defaults.HTTPHostHeader != "" {
		b.config.HTTPHostHeader = &defaults.HTTPHostHeader
	}

	if defaults.ProxyAddress != "" {
		b.config.ProxyAddress = &defaults.ProxyAddress
	}

	if defaults.ProxyPort != nil {
		p := uint(*defaults.ProxyPort)
		b.config.ProxyPort = &p
	}

	if defaults.ProxyType != "" {
		b.config.ProxyType = &defaults.ProxyType
	}

	if defaults.DisableChunkedEncoding != nil {
		b.config.DisableChunkedEncoding = defaults.DisableChunkedEncoding
	}

	if defaults.BastionMode != nil {
		b.config.BastionMode = defaults.BastionMode
	}

	return b
}

// SetNoTLSVerify sets the NoTLSVerify option.
func (b *OriginRequestBuilder) SetNoTLSVerify(v *bool) *OriginRequestBuilder {
	if v != nil {
		b.config.NoTLSVerify = v
	}
	return b
}

// SetHTTP2Origin sets the HTTP2Origin option.
func (b *OriginRequestBuilder) SetHTTP2Origin(v *bool) *OriginRequestBuilder {
	if v != nil {
		b.config.Http2Origin = v
	}
	return b
}

// SetCAPool sets the CAPool option (converts to path).
func (b *OriginRequestBuilder) SetCAPool(caPool string) *OriginRequestBuilder {
	if caPool != "" {
		caPath := fmt.Sprintf("/etc/cloudflared/certs/%s", caPool)
		b.config.CAPool = &caPath
	}
	return b
}

// SetConnectTimeout sets the ConnectTimeout option.
func (b *OriginRequestBuilder) SetConnectTimeout(d *time.Duration) *OriginRequestBuilder {
	if d != nil {
		b.config.ConnectTimeout = d
	}
	return b
}

// SetTLSTimeout sets the TLSTimeout option.
func (b *OriginRequestBuilder) SetTLSTimeout(d *time.Duration) *OriginRequestBuilder {
	if d != nil {
		b.config.TLSTimeout = d
	}
	return b
}

// SetKeepAliveTimeout sets the KeepAliveTimeout option.
func (b *OriginRequestBuilder) SetKeepAliveTimeout(d *time.Duration) *OriginRequestBuilder {
	if d != nil {
		b.config.KeepAliveTimeout = d
	}
	return b
}

// SetKeepAliveConnections sets the KeepAliveConnections option.
func (b *OriginRequestBuilder) SetKeepAliveConnections(n *int) *OriginRequestBuilder {
	if n != nil {
		b.config.KeepAliveConnections = n
	}
	return b
}

// SetOriginServerName sets the OriginServerName option.
func (b *OriginRequestBuilder) SetOriginServerName(v string) *OriginRequestBuilder {
	if v != "" {
		b.config.OriginServerName = &v
	}
	return b
}

// SetHTTPHostHeader sets the HTTPHostHeader option.
func (b *OriginRequestBuilder) SetHTTPHostHeader(v string) *OriginRequestBuilder {
	if v != "" {
		b.config.HTTPHostHeader = &v
	}
	return b
}

// SetProxyAddress sets the ProxyAddress option.
func (b *OriginRequestBuilder) SetProxyAddress(v string) *OriginRequestBuilder {
	if v != "" {
		b.config.ProxyAddress = &v
	}
	return b
}

// SetProxyPort sets the ProxyPort option.
func (b *OriginRequestBuilder) SetProxyPort(port uint) *OriginRequestBuilder {
	if port != 0 {
		b.config.ProxyPort = &port
	}
	return b
}

// SetProxyType sets the ProxyType option.
func (b *OriginRequestBuilder) SetProxyType(v string) *OriginRequestBuilder {
	if v != "" {
		b.config.ProxyType = &v
	}
	return b
}

// SetDisableChunkedEncoding sets the DisableChunkedEncoding option.
func (b *OriginRequestBuilder) SetDisableChunkedEncoding(v *bool) *OriginRequestBuilder {
	if v != nil {
		b.config.DisableChunkedEncoding = v
	}
	return b
}

// SetBastionMode sets the BastionMode option.
func (b *OriginRequestBuilder) SetBastionMode(v *bool) *OriginRequestBuilder {
	if v != nil {
		b.config.BastionMode = v
	}
	return b
}

// Build returns the constructed OriginRequestConfig.
func (b *OriginRequestBuilder) Build() cf.OriginRequestConfig {
	return b.config
}
