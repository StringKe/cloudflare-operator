// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package framework provides the E2E test framework for cloudflare-operator.
// It manages Kind cluster lifecycle, mock server, and test utilities.
package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
	"github.com/StringKe/cloudflare-operator/test/mockserver"
)

const (
	// DefaultTimeout is the default timeout for operations
	DefaultTimeout = 5 * time.Minute
	// DefaultInterval is the default polling interval
	DefaultInterval = 2 * time.Second
	// KindClusterName is the name of the Kind cluster
	KindClusterName = "cloudflare-operator-e2e"
	// OperatorNamespace is the namespace where the operator is deployed
	OperatorNamespace = "cloudflare-operator-system"
	// TestNamespace is the default namespace for E2E tests
	TestNamespace = "e2e-test"
)

// Framework provides utilities for E2E testing
type Framework struct {
	Client         client.Client
	MockServer     *mockserver.Server
	KubeconfigPath string
	ClusterCreated bool
	ctx            context.Context
	cancel         context.CancelFunc
}

// Options configures the test framework
type Options struct {
	// UseExistingCluster uses an existing cluster instead of creating a Kind cluster
	UseExistingCluster bool
	// KubeconfigPath is the path to the kubeconfig file
	KubeconfigPath string
	// MockServerPort is the port for the mock Cloudflare API server
	MockServerPort int
	// SkipMockServer skips starting the mock server
	SkipMockServer bool
}

// DefaultOptions returns default framework options
func DefaultOptions() *Options {
	return &Options{
		UseExistingCluster: os.Getenv("USE_EXISTING_CLUSTER") == "true",
		KubeconfigPath:     os.Getenv("KUBECONFIG"),
		MockServerPort:     8787,
		SkipMockServer:     false,
	}
}

// New creates a new test framework
func New(opts *Options) (*Framework, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())
	f := &Framework{
		ctx:    ctx,
		cancel: cancel,
	}

	// Start mock server
	if !opts.SkipMockServer {
		f.MockServer = mockserver.NewServer(mockserver.WithPort(opts.MockServerPort))
		if err := f.MockServer.StartAsync(); err != nil {
			cancel()
			return nil, fmt.Errorf("start mock server: %w", err)
		}
	}

	// Create or use existing cluster
	if !opts.UseExistingCluster {
		if err := f.createKindCluster(); err != nil {
			f.Cleanup()
			return nil, fmt.Errorf("create kind cluster: %w", err)
		}
		f.ClusterCreated = true
		f.KubeconfigPath = filepath.Join(os.TempDir(), "cloudflare-operator-e2e-kubeconfig")
	} else {
		f.KubeconfigPath = opts.KubeconfigPath
		if f.KubeconfigPath == "" {
			f.KubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
	}

	// Create Kubernetes client
	if err := f.createClient(); err != nil {
		f.Cleanup()
		return nil, fmt.Errorf("create client: %w", err)
	}

	return f, nil
}

// createKindCluster creates a Kind cluster for E2E testing
func (f *Framework) createKindCluster() error {
	// Check if cluster exists
	checkCmd := exec.Command("kind", "get", "clusters")
	output, err := checkCmd.Output()
	if err == nil && strings.Contains(string(output), KindClusterName) {
		fmt.Printf("Kind cluster %s already exists\n", KindClusterName)
		return nil
	}

	// Create cluster
	fmt.Printf("Creating Kind cluster %s...\n", KindClusterName)
	configPath := filepath.Join("test", "e2e", "config", "kind-config.yaml")

	args := []string{"create", "cluster", "--name", KindClusterName}
	if _, err := os.Stat(configPath); err == nil {
		args = append(args, "--config", configPath)
	}

	createCmd := exec.Command("kind", args...)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster: %w", err)
	}

	// Get kubeconfig
	kubeconfigPath := filepath.Join(os.TempDir(), "cloudflare-operator-e2e-kubeconfig")
	getCmd := exec.Command("kind", "get", "kubeconfig", "--name", KindClusterName)
	kubeconfig, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("get kubeconfig: %w", err)
	}
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}

	return nil
}

// createClient creates a Kubernetes client
func (f *Framework) createClient() error {
	config, err := clientcmd.BuildConfigFromFlags("", f.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	// Register CRDs
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return fmt.Errorf("add core scheme: %w", err)
	}
	if err := v1alpha2.AddToScheme(s); err != nil {
		return fmt.Errorf("add v1alpha2 scheme: %w", err)
	}

	f.Client, err = client.New(config, client.Options{Scheme: s})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	return nil
}

// Cleanup cleans up the test framework resources
func (f *Framework) Cleanup() {
	if f.cancel != nil {
		f.cancel()
	}

	if f.MockServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = f.MockServer.Stop(ctx)
	}

	if f.ClusterCreated {
		fmt.Printf("Deleting Kind cluster %s...\n", KindClusterName)
		deleteCmd := exec.Command("kind", "delete", "cluster", "--name", KindClusterName)
		_ = deleteCmd.Run()
	}
}

// SetupTestNamespace creates a test namespace
func (f *Framework) SetupTestNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := f.Client.Create(f.ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}

	return nil
}

// CleanupTestNamespace deletes a test namespace
func (f *Framework) CleanupTestNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := f.Client.Delete(f.ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}

	return nil
}

// CreateSecret creates a secret in the specified namespace
func (f *Framework) CreateSecret(namespace, name string, data map[string]string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: data,
	}

	err := f.Client.Create(f.ctx, secret)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create secret %s/%s: %w", namespace, name, err)
	}

	return nil
}

// WaitForCondition waits for a condition to be true on a resource
func (f *Framework) WaitForCondition(
	obj client.Object,
	conditionType string,
	expectedStatus metav1.ConditionStatus,
	timeout time.Duration,
) error {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	return wait.PollUntilContextTimeout(f.ctx, DefaultInterval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := f.Client.Get(ctx, key, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		// Get conditions from status
		conditions := getConditions(obj)
		for _, cond := range conditions {
			if cond.Type == conditionType && cond.Status == expectedStatus {
				return true, nil
			}
		}

		return false, nil
	})
}

// WaitForDeletion waits for a resource to be deleted
func (f *Framework) WaitForDeletion(obj client.Object, timeout time.Duration) error {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	return wait.PollUntilContextTimeout(f.ctx, DefaultInterval, timeout, true, func(ctx context.Context) (bool, error) {
		err := f.Client.Get(ctx, key, obj)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
}

// getConditions extracts conditions from a resource status
func getConditions(obj client.Object) []metav1.Condition {
	switch typed := obj.(type) {
	case *v1alpha2.Tunnel:
		return typed.Status.Conditions
	case *v1alpha2.ClusterTunnel:
		return typed.Status.Conditions
	case *v1alpha2.DNSRecord:
		return typed.Status.Conditions
	case *v1alpha2.VirtualNetwork:
		return typed.Status.Conditions
	case *v1alpha2.NetworkRoute:
		return typed.Status.Conditions
	case *v1alpha2.AccessApplication:
		return typed.Status.Conditions
	case *v1alpha2.AccessGroup:
		return typed.Status.Conditions
	default:
		return nil
	}
}

// Context returns the framework context
func (f *Framework) Context() context.Context {
	return f.ctx
}

// ResetMockServer resets the mock server state
func (f *Framework) ResetMockServer() {
	if f.MockServer != nil {
		f.MockServer.Reset()
	}
}

// MockServerURL returns the mock server URL
func (f *Framework) MockServerURL() string {
	if f.MockServer != nil {
		return f.MockServer.URL()
	}
	return ""
}
