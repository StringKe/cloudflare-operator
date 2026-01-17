// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package testutil

import (
	"context"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

const (
	testTimeout  = 30 * time.Second
	testInterval = 250 * time.Millisecond
)

// TestEnv wraps envtest for controller testing.
type TestEnv struct {
	Env        *envtest.Environment
	Cfg        *rest.Config
	K8sClient  client.Client
	Ctx        context.Context
	Cancel     context.CancelFunc
	Mgr        ctrl.Manager
	MgrStarted bool
}

// TestEnvOptions configures the test environment.
type TestEnvOptions struct {
	// CRDPaths are paths to CRD manifests
	CRDPaths []string
	// UseExistingCluster connects to an existing cluster instead of starting envtest
	UseExistingCluster bool
	// StartManager whether to start a controller manager
	StartManager bool
}

// DefaultTestEnvOptions returns default options.
func DefaultTestEnvOptions() *TestEnvOptions {
	return &TestEnvOptions{
		CRDPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		UseExistingCluster: false,
		StartManager:       false,
	}
}

// NewTestEnv creates a new test environment.
func NewTestEnv(opts *TestEnvOptions) (*TestEnv, error) {
	if opts == nil {
		opts = DefaultTestEnvOptions()
	}

	// Setup logging
	logf.SetLogger(zap.New(zap.WriteTo(nil), zap.UseDevMode(true)))

	// Create envtest environment
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     opts.CRDPaths,
		ErrorIfCRDPathMissing: true,
	}

	if opts.UseExistingCluster {
		useExisting := true
		testEnv.UseExistingCluster = &useExisting
	}

	// Start the environment
	cfg, err := testEnv.Start()
	if err != nil {
		return nil, err
	}

	// Register schemes
	if err := v1alpha2.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	// Create client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	te := &TestEnv{
		Env:       testEnv,
		Cfg:       cfg,
		K8sClient: k8sClient,
		Ctx:       ctx,
		Cancel:    cancel,
	}

	// Optionally create and start manager
	if opts.StartManager {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
		})
		if err != nil {
			cancel()
			testEnv.Stop()
			return nil, err
		}
		te.Mgr = mgr

		go func() {
			if err := mgr.Start(ctx); err != nil {
				logf.Log.Error(err, "manager exited with error")
			}
		}()
		te.MgrStarted = true
	}

	return te, nil
}

// Stop stops the test environment.
func (te *TestEnv) Stop() error {
	if te.Cancel != nil {
		te.Cancel()
	}
	if te.Env != nil {
		return te.Env.Stop()
	}
	return nil
}

// CreateNamespace creates a namespace in the test cluster.
func (te *TestEnv) CreateNamespace(name string) error {
	ns := NewFixtures().WithNamespace(name).TestNamespaceObj()
	return te.K8sClient.Create(te.Ctx, ns)
}

// DeleteNamespace deletes a namespace from the test cluster.
func (te *TestEnv) DeleteNamespace(name string) error {
	ns := NewFixtures().WithNamespace(name).TestNamespaceObj()
	return te.K8sClient.Delete(te.Ctx, ns)
}

// EnsureNamespace ensures a namespace exists.
func (te *TestEnv) EnsureNamespace(name string) error {
	ns := NewFixtures().WithNamespace(name).TestNamespaceObj()
	err := te.K8sClient.Create(te.Ctx, ns)
	if err != nil && client.IgnoreAlreadyExists(err) != nil {
		return err
	}
	return nil
}

// CleanupResources deletes all resources of a type in a namespace.
// The template parameter should be an empty object of the type to delete.
func (te *TestEnv) CleanupResources(template client.Object, namespace string) error {
	return te.K8sClient.DeleteAllOf(te.Ctx, template, client.InNamespace(namespace))
}
