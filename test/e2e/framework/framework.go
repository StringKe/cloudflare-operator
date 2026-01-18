// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

// Package framework provides the E2E test framework for cloudflare-operator.
// It manages Kind cluster lifecycle, mock server, and test utilities.
package framework

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	// MockServerNamespace is the namespace where the mock server is deployed
	MockServerNamespace = "cloudflare-mock"
	// MockServerName is the name of the mock server deployment/service
	MockServerName = "mockserver"
	// MockServerImage is the Docker image for the mock server
	MockServerImage = "cloudflare-mockserver:e2e-test"
	// MockServerPort is the port the mock server listens on
	MockServerPort = 8787
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
	// MockServerPort is the port for the mock Cloudflare API server (deprecated, use in-cluster mock)
	MockServerPort int
	// SkipMockServer skips starting the mock server (deprecated, use UseInClusterMockServer)
	SkipMockServer bool
	// UseInClusterMockServer deploys mock server to cluster instead of running locally
	UseInClusterMockServer bool
	// SkipMockServerSetup skips all mock server setup (for tests using real API)
	SkipMockServerSetup bool
}

// DefaultOptions returns default framework options
func DefaultOptions() *Options {
	return &Options{
		UseExistingCluster:     os.Getenv("USE_EXISTING_CLUSTER") == "true",
		KubeconfigPath:         os.Getenv("KUBECONFIG"),
		MockServerPort:         8787,
		SkipMockServer:         false,
		UseInClusterMockServer: true, // Default to in-cluster mock server
		SkipMockServerSetup:    false,
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

	// Setup mock server
	if !opts.SkipMockServerSetup {
		if opts.UseInClusterMockServer {
			// Deploy mock server to cluster (for E2E tests)
			// This is handled separately by SetupMockServerInCluster()
			// because it requires the cluster to be ready first
			_, _ = fmt.Fprintln(os.Stdout, "In-cluster mock server mode enabled. Call SetupMockServerInCluster() to deploy.")
		} else if !opts.SkipMockServer {
			// Legacy: Start local mock server (for local development only)
			f.MockServer = mockserver.NewServer(mockserver.WithPort(opts.MockServerPort))
			if err := f.MockServer.StartAsync(); err != nil {
				f.Cleanup()
				return nil, fmt.Errorf("start mock server: %w", err)
			}
		}
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
	// Network Layer
	case *v1alpha2.Tunnel:
		return typed.Status.Conditions
	case *v1alpha2.ClusterTunnel:
		return typed.Status.Conditions
	case *v1alpha2.VirtualNetwork:
		return typed.Status.Conditions
	case *v1alpha2.NetworkRoute:
		return typed.Status.Conditions
	case *v1alpha2.WARPConnector:
		return typed.Status.Conditions
	// Service Layer
	case *v1alpha2.PrivateService:
		return typed.Status.Conditions
	case *v1alpha2.DNSRecord:
		return typed.Status.Conditions
	// Access Layer
	case *v1alpha2.AccessApplication:
		return typed.Status.Conditions
	case *v1alpha2.AccessGroup:
		return typed.Status.Conditions
	case *v1alpha2.AccessIdentityProvider:
		return typed.Status.Conditions
	case *v1alpha2.AccessServiceToken:
		return typed.Status.Conditions
	// Device Layer
	case *v1alpha2.DevicePostureRule:
		return typed.Status.Conditions
	case *v1alpha2.DeviceSettingsPolicy:
		return typed.Status.Conditions
	// Gateway Layer
	case *v1alpha2.GatewayRule:
		return typed.Status.Conditions
	case *v1alpha2.GatewayList:
		return typed.Status.Conditions
	case *v1alpha2.GatewayConfiguration:
		return typed.Status.Conditions
	// R2 Storage
	case *v1alpha2.R2Bucket:
		return typed.Status.Conditions
	case *v1alpha2.R2BucketDomain:
		return typed.Status.Conditions
	case *v1alpha2.R2BucketNotification:
		return typed.Status.Conditions
	// Rules
	case *v1alpha2.ZoneRuleset:
		return typed.Status.Conditions
	case *v1alpha2.TransformRule:
		return typed.Status.Conditions
	case *v1alpha2.RedirectRule:
		return typed.Status.Conditions
	// SSL/TLS
	case *v1alpha2.OriginCACertificate:
		return typed.Status.Conditions
	// Credentials
	case *v1alpha2.CloudflareCredentials:
		return typed.Status.Conditions
	// Domain
	case *v1alpha2.CloudflareDomain:
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

// WaitForOperatorReady waits for the operator deployment to be available
func (f *Framework) WaitForOperatorReady(timeout time.Duration) error {
	return wait.PollUntilContextTimeout(f.ctx, DefaultInterval, timeout, true, func(ctx context.Context) (bool, error) {
		var deploy appsv1.Deployment
		key := types.NamespacedName{
			Name:      "cloudflare-operator-controller-manager",
			Namespace: OperatorNamespace,
		}
		if err := f.Client.Get(ctx, key, &deploy); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return deploy.Status.AvailableReplicas > 0, nil
	})
}

// CreateCloudflareCredentials creates a CloudflareCredentials resource and its secret
func (f *Framework) CreateCloudflareCredentials(name string, apiToken string, accountID string, isDefault bool) error {
	// Create secret first
	secretNS := OperatorNamespace
	secretName := name + "-secret"

	if err := f.SetupTestNamespace(secretNS); err != nil {
		return fmt.Errorf("create operator namespace: %w", err)
	}

	if err := f.CreateSecret(secretNS, secretName, map[string]string{
		"CLOUDFLARE_API_TOKEN": apiToken,
	}); err != nil {
		return fmt.Errorf("create credentials secret: %w", err)
	}

	// Create CloudflareCredentials
	creds := &v1alpha2.CloudflareCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.CloudflareCredentialsSpec{
			AuthType:  v1alpha2.AuthTypeAPIToken,
			AccountID: accountID,
			SecretRef: v1alpha2.SecretReference{
				Name:      secretName,
				Namespace: secretNS,
			},
			IsDefault: isDefault,
		},
	}

	err := f.Client.Create(f.ctx, creds)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create cloudflare credentials: %w", err)
	}

	return nil
}

// WaitForStatusField waits for a specific status field to have an expected value
func (f *Framework) WaitForStatusField(
	obj client.Object,
	fieldChecker func(client.Object) bool,
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
		return fieldChecker(obj), nil
	})
}

// EnsureNamespaceExists creates a namespace if it doesn't exist
func (f *Framework) EnsureNamespaceExists(name string) error {
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

// CreateTestService creates a test deployment and service for TunnelBinding tests
func (f *Framework) CreateTestService(namespace, name string, port int32) error {
	// Create Deployment
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:alpine",
							Ports: []corev1.ContainerPort{
								{ContainerPort: port},
							},
						},
					},
				},
			},
		},
	}

	if err := f.Client.Create(f.ctx, deploy); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create deployment: %w", err)
	}

	// Create Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				},
			},
		},
	}

	if err := f.Client.Create(f.ctx, svc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create service: %w", err)
	}

	return nil
}

// DeleteResource deletes a resource without waiting
func (f *Framework) DeleteResource(obj client.Object) error {
	err := f.Client.Delete(f.ctx, obj)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete resource: %w", err)
	}
	return nil
}

// DeleteResourceAndWait deletes a resource and waits for deletion to complete
func (f *Framework) DeleteResourceAndWait(obj client.Object, timeout time.Duration) error {
	if err := f.DeleteResource(obj); err != nil {
		return err
	}
	return f.WaitForDeletion(obj, timeout)
}

// GetResource fetches a resource by name and namespace
func (f *Framework) GetResource(obj client.Object, name, namespace string) error {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	return f.Client.Get(f.ctx, key, obj)
}

// BuildMockServerImage builds the mock server Docker image
func (*Framework) BuildMockServerImage() error {
	_, _ = fmt.Fprintln(os.Stdout, "Building mock server Docker image...")

	// Build from project root
	buildCmd := exec.Command("docker", "build",
		"-t", MockServerImage,
		"-f", "test/mockserver/Dockerfile",
		".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build mock server image: %w", err)
	}

	return nil
}

// LoadMockServerImageToKind loads the mock server image into Kind
func (*Framework) LoadMockServerImageToKind() error {
	_, _ = fmt.Fprintln(os.Stdout, "Loading mock server image into Kind cluster...")

	loadCmd := exec.Command("kind", "load", "docker-image",
		MockServerImage,
		"--name", KindClusterName)
	loadCmd.Stdout = os.Stdout
	loadCmd.Stderr = os.Stderr

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("load image to kind: %w", err)
	}

	return nil
}

// DeployMockServerToCluster deploys the mock server as a Kubernetes deployment
func (f *Framework) DeployMockServerToCluster() error {
	_, _ = fmt.Fprintln(os.Stdout, "Deploying mock server to cluster...")

	// Create namespace
	if err := f.EnsureNamespaceExists(MockServerNamespace); err != nil {
		return fmt.Errorf("create mock server namespace: %w", err)
	}

	// Create deployment
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockServerName,
			Namespace: MockServerNamespace,
			Labels:    map[string]string{"app": "cloudflare-mockserver"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "cloudflare-mockserver"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "cloudflare-mockserver"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "mockserver",
							Image:           MockServerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{ContainerPort: MockServerPort, Name: "http"},
							},
							Args: []string{fmt.Sprintf("-port=%d", MockServerPort)},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(MockServerPort),
									},
								},
								InitialDelaySeconds: 2,
								PeriodSeconds:       5,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(MockServerPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
				},
			},
		},
	}

	if err := f.Client.Create(f.ctx, deploy); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create mock server deployment: %w", err)
	}

	// Create service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockServerName,
			Namespace: MockServerNamespace,
			Labels:    map[string]string{"app": "cloudflare-mockserver"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "cloudflare-mockserver"},
			Ports: []corev1.ServicePort{
				{
					Port:       MockServerPort,
					TargetPort: intstr.FromInt32(MockServerPort),
					Name:       "http",
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := f.Client.Create(f.ctx, svc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create mock server service: %w", err)
	}

	// Wait for deployment to be ready
	return f.WaitForMockServerReady(DefaultTimeout)
}

// WaitForMockServerReady waits for the mock server deployment to be available
func (f *Framework) WaitForMockServerReady(timeout time.Duration) error {
	_, _ = fmt.Fprintln(os.Stdout, "Waiting for mock server to be ready...")

	return wait.PollUntilContextTimeout(f.ctx, DefaultInterval, timeout, true, func(ctx context.Context) (bool, error) {
		var deploy appsv1.Deployment
		key := types.NamespacedName{
			Name:      MockServerName,
			Namespace: MockServerNamespace,
		}
		if err := f.Client.Get(ctx, key, &deploy); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return deploy.Status.AvailableReplicas > 0 && deploy.Status.ReadyReplicas > 0, nil
	})
}

// InClusterMockServerURL returns the in-cluster URL for the mock server
func (*Framework) InClusterMockServerURL() string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		MockServerName, MockServerNamespace, MockServerPort)
}

// SetupMockServerInCluster builds, loads, and deploys the mock server to the cluster
func (f *Framework) SetupMockServerInCluster() error {
	if err := f.BuildMockServerImage(); err != nil {
		return err
	}
	if err := f.LoadMockServerImageToKind(); err != nil {
		return err
	}
	return f.DeployMockServerToCluster()
}

// setContainerEnvVar sets or updates an environment variable in a container
func setContainerEnvVar(container *corev1.Container, name, value string) {
	for i := range container.Env {
		if container.Env[i].Name == name {
			container.Env[i].Value = value
			return
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
}

// findManagerContainer finds the manager container in a pod spec
func findManagerContainer(podSpec *corev1.PodSpec) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "manager" {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

// PatchOperatorWithMockServerURL patches the operator deployment to use the mock server URL
func (f *Framework) PatchOperatorWithMockServerURL() error {
	_, _ = fmt.Fprintln(os.Stdout, "Patching operator to use mock server URL...")

	var deploy appsv1.Deployment
	key := types.NamespacedName{
		Name:      "cloudflare-operator-controller-manager",
		Namespace: OperatorNamespace,
	}

	if err := f.Client.Get(f.ctx, key, &deploy); err != nil {
		return fmt.Errorf("get operator deployment: %w", err)
	}

	container := findManagerContainer(&deploy.Spec.Template.Spec)
	if container == nil {
		return errors.New("manager container not found in operator deployment")
	}

	setContainerEnvVar(container, "CLOUDFLARE_API_BASE_URL", f.InClusterMockServerURL())

	if err := f.Client.Update(f.ctx, &deploy); err != nil {
		return fmt.Errorf("update operator deployment: %w", err)
	}

	_, _ = fmt.Fprintln(os.Stdout, "Waiting for operator to restart with new configuration...")
	time.Sleep(5 * time.Second) // Give time for rollout to start

	return f.WaitForOperatorReady(DefaultTimeout)
}
