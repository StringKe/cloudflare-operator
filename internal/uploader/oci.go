// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package uploader

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// OCIUploader downloads files from OCI registries.
type OCIUploader struct {
	imageRef name.Reference
	options  []remote.Option
}

// NewOCIUploader creates a new OCI uploader from configuration.
//
//nolint:revive // cognitive complexity is acceptable for this configuration function
func NewOCIUploader(ctx context.Context, k8sClient client.Client, namespace string, cfg *v1alpha2.OCISource) (*OCIUploader, error) {
	if cfg == nil {
		return nil, errors.New("OCI source config is nil")
	}

	if cfg.Image == "" {
		return nil, errors.New("OCI image reference is required")
	}

	// Parse image reference
	var nameOpts []name.Option
	if cfg.InsecureRegistry {
		nameOpts = append(nameOpts, name.Insecure)
	}

	ref, err := name.ParseReference(cfg.Image, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("parse image reference %q: %w", cfg.Image, err)
	}

	// Build remote options
	var remoteOpts []remote.Option
	remoteOpts = append(remoteOpts, remote.WithContext(ctx))

	// Load credentials from secret if specified
	if cfg.CredentialsSecretRef != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      cfg.CredentialsSecretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("get credentials secret %q: %w", cfg.CredentialsSecretRef.Name, err)
		}

		auth, err := parseRegistryAuth(secret)
		if err != nil {
			return nil, fmt.Errorf("parse registry auth: %w", err)
		}

		remoteOpts = append(remoteOpts, remote.WithAuth(auth))
	}

	return &OCIUploader{
		imageRef: ref,
		options:  remoteOpts,
	}, nil
}

// Download fetches the artifact from the OCI registry.
// For OCI artifacts, this returns the first layer of the image.
func (u *OCIUploader) Download(_ context.Context) (io.ReadCloser, error) {
	// Note: Context is already embedded in u.options via remote.WithContext() in NewOCIUploader

	// Get the image
	img, err := remote.Image(u.imageRef, u.options...)
	if err != nil {
		return nil, fmt.Errorf("fetch OCI image %s: %w", u.imageRef.String(), err)
	}

	// Get layers
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("get image layers: %w", err)
	}

	if len(layers) == 0 {
		return nil, errors.New("image has no layers")
	}

	// Return the first layer (typically contains the artifact data)
	reader, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("uncompress layer: %w", err)
	}

	return reader, nil
}

// GetContentType returns the expected content type.
func (*OCIUploader) GetContentType() string {
	return ContentTypeOctetStream
}

// parseRegistryAuth parses registry authentication from a Kubernetes secret.
// Supports both Docker config format (.dockerconfigjson) and basic auth (username/password).
func parseRegistryAuth(secret *corev1.Secret) (authn.Authenticator, error) {
	// Try Docker config format first
	if dockerConfigJSON, ok := secret.Data[".dockerconfigjson"]; ok {
		return parseDockerConfig(dockerConfigJSON)
	}

	// Fall back to basic auth
	username := string(secret.Data["username"])
	password := string(secret.Data["password"])

	if username == "" || password == "" {
		return nil, errors.New("secret must contain .dockerconfigjson or username/password")
	}

	return &authn.Basic{
		Username: username,
		Password: password,
	}, nil
}

// dockerConfig represents the Docker config.json format.
type dockerConfig struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

// parseDockerConfig parses a Docker config.json and returns an authenticator.
//
//nolint:revive // cognitive complexity is acceptable for this parsing function
func parseDockerConfig(data []byte) (authn.Authenticator, error) {
	var config dockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse docker config: %w", err)
	}

	// Return the first auth entry found
	for _, entry := range config.Auths {
		if entry.Auth != "" {
			// Decode base64 auth string (username:password)
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				return nil, fmt.Errorf("decode auth string: %w", err)
			}

			// Split into username:password
			for i := 0; i < len(decoded); i++ {
				if decoded[i] == ':' {
					return &authn.Basic{
						Username: string(decoded[:i]),
						Password: string(decoded[i+1:]),
					}, nil
				}
			}

			return nil, errors.New("invalid auth string format")
		}

		if entry.Username != "" && entry.Password != "" {
			return &authn.Basic{
				Username: entry.Username,
				Password: entry.Password,
			}, nil
		}
	}

	return nil, errors.New("no valid auth entries found in docker config")
}
