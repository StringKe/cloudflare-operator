// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package uploader

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// S3Uploader downloads files from S3-compatible storage.
type S3Uploader struct {
	s3Client *s3.Client
	bucket   string
	key      string
}

// NewS3Uploader creates a new S3 uploader from configuration.
//
//nolint:revive // cognitive complexity is acceptable for this configuration function
func NewS3Uploader(ctx context.Context, k8sClient client.Client, namespace string, cfg *v1alpha2.S3Source) (*S3Uploader, error) {
	if cfg == nil {
		return nil, errors.New("S3 source config is nil")
	}

	if cfg.Bucket == "" {
		return nil, errors.New("S3 bucket is required")
	}

	if cfg.Key == "" {
		return nil, errors.New("S3 key is required")
	}

	// Build AWS config options
	var opts []func(*config.LoadOptions) error

	// Set region if specified
	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	// Load credentials from secret if specified
	if cfg.CredentialsSecretRef != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      cfg.CredentialsSecretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("get credentials secret %q: %w", cfg.CredentialsSecretRef.Name, err)
		}

		accessKeyID := string(secret.Data["accessKeyId"])
		secretAccessKey := string(secret.Data["secretAccessKey"])
		sessionToken := string(secret.Data["sessionToken"])

		if accessKeyID == "" || secretAccessKey == "" {
			return nil, errors.New("credentials secret must contain accessKeyId and secretAccessKey")
		}

		creds := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken)
		opts = append(opts, config.WithCredentialsProvider(creds))
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	// Build S3 client options
	var s3Opts []func(*s3.Options)

	// Set custom endpoint if specified (for S3-compatible services)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	// Enable path-style addressing if specified
	if cfg.UsePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Uploader{
		s3Client: s3Client,
		bucket:   cfg.Bucket,
		key:      cfg.Key,
	}, nil
}

// Download fetches the file from S3.
func (u *S3Uploader) Download(ctx context.Context) (io.ReadCloser, error) {
	output, err := u.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(u.key),
	})
	if err != nil {
		return nil, fmt.Errorf("get S3 object s3://%s/%s: %w", u.bucket, u.key, err)
	}

	return output.Body, nil
}

// GetContentType returns the expected content type.
func (*S3Uploader) GetContentType() string {
	return ContentTypeOctetStream
}
