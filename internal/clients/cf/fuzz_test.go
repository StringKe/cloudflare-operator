// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package cf

import (
	"testing"

	"sigs.k8s.io/yaml"
)

// FuzzConfigurationUnmarshal tests YAML unmarshaling with fuzz data
func FuzzConfigurationUnmarshal(f *testing.F) {
	// Seed corpus with valid configurations
	f.Add([]byte(`tunnel: abc-123
credentials-file: /path/to/creds
ingress:
  - hostname: test.example.com
    service: http://test:80
  - service: http_status:404`))

	f.Add([]byte(`tunnel: xyz
warp-routing:
  enabled: true
ingress:
  - service: http_status:404`))

	f.Add([]byte(`{}`))
	f.Add([]byte(`tunnel: ""`))
	f.Add([]byte(`ingress: []`))

	f.Fuzz(func(_ *testing.T, data []byte) {
		var config Configuration
		// Should not panic on any input
		_ = yaml.Unmarshal(data, &config)
	})
}

// FuzzConfigurationRoundTrip tests marshal/unmarshal round trip
// nolint:revive // cognitive complexity is acceptable for fuzz tests
func FuzzConfigurationRoundTrip(f *testing.F) {
	f.Add("tunnel-id", "creds.json", "0.0.0.0:2000", true, "test.example.com", "http://test:80")

	f.Fuzz(func(t *testing.T, tunnelId, sourceFile, metrics string, warpEnabled bool, hostname, service string) {
		config := &Configuration{
			TunnelId:   tunnelId,
			SourceFile: sourceFile,
			Metrics:    metrics,
			WarpRouting: WarpRoutingConfig{
				Enabled: warpEnabled,
			},
			Ingress: []UnvalidatedIngressRule{
				{
					Hostname: hostname,
					Service:  service,
				},
			},
		}

		// Marshal
		data, err := yaml.Marshal(config)
		if err != nil {
			return // Skip invalid configurations
		}

		// Unmarshal
		var restored Configuration
		if err := yaml.Unmarshal(data, &restored); err != nil {
			t.Errorf("Failed to unmarshal marshaled config: %v", err)
			return
		}

		// Verify critical fields are preserved
		if restored.TunnelId != tunnelId {
			t.Errorf("TunnelId mismatch: got %q, want %q", restored.TunnelId, tunnelId)
		}
		if restored.SourceFile != sourceFile {
			t.Errorf("SourceFile mismatch: got %q, want %q", restored.SourceFile, sourceFile)
		}
	})
}

// FuzzSanitizeErrorMessage tests error message sanitization with fuzz data
// nolint:revive // cognitive complexity is acceptable for fuzz tests
func FuzzSanitizeErrorMessage(f *testing.F) {
	// Seed with various error messages
	f.Add("simple error")
	f.Add("error with token: abc123")
	f.Add("secret value: hunter2")
	f.Add("password: mypassword123")
	f.Add("API key: sk-1234567890")
	f.Add("bearer: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	f.Add("normal error without sensitive data")
	f.Add("") // empty string

	f.Fuzz(func(t *testing.T, errMsg string) {
		if errMsg == "" {
			return
		}

		err := &testError{msg: errMsg}
		sanitized := SanitizeErrorMessage(err)

		// Should never panic
		// Should never return the original if it contains sensitive patterns
		sensitivePatterns := []string{"token", "secret", "password", "credential", "api_key", "apikey", "bearer", "authorization"}

		for _, pattern := range sensitivePatterns {
			if containsIgnoreCase(errMsg, pattern) {
				// If original contains sensitive pattern, sanitized should not
				if containsIgnoreCase(sanitized, pattern) {
					t.Errorf("Sanitized message still contains sensitive pattern %q", pattern)
				}
			}
		}

		// Should be limited in length
		if len(sanitized) > 512 {
			t.Errorf("Sanitized message too long: %d bytes", len(sanitized))
		}
	})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// nolint:revive // cognitive complexity is acceptable for test helpers
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive contains check
	for i := 0; i <= len(s)-len(substr); i++ {
		found := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			// Convert to lowercase
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 32
			}
			if c1 != c2 {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

// FuzzIsNotFoundError tests error classification with fuzz data
func FuzzIsNotFoundError(f *testing.F) {
	f.Add("not found")
	f.Add("resource not found")
	f.Add("tunnel does not exist")
	f.Add("no such record")
	f.Add("HTTP 404")
	f.Add("random error message")
	f.Add("")

	f.Fuzz(func(_ *testing.T, errMsg string) {
		if errMsg == "" {
			return
		}

		err := &testError{msg: errMsg}
		// Should not panic
		_ = IsNotFoundError(err)
	})
}

// FuzzIsTemporaryError tests temporary error classification
func FuzzIsTemporaryError(f *testing.F) {
	f.Add("timeout")
	f.Add("connection refused")
	f.Add("temporary failure")
	f.Add("rate limit exceeded")
	f.Add("HTTP 503")
	f.Add("HTTP 502")
	f.Add("HTTP 504")
	f.Add("permanent error")

	f.Fuzz(func(_ *testing.T, errMsg string) {
		if errMsg == "" {
			return
		}

		err := &testError{msg: errMsg}
		// Should not panic
		_ = IsTemporaryError(err)
	})
}
