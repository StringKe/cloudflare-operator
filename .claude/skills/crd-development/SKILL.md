---
name: crd-development
description: Develop new Kubernetes CRD for Cloudflare Operator. Use when creating new CRD types, implementing controllers, or adding Cloudflare API integrations. Triggers on "add CRD", "new resource", "implement controller".
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

# CRD Development Guide

## Overview

This skill guides the development of new Custom Resource Definitions (CRDs) for the Cloudflare Operator, following established patterns and best practices.

## Project Structure

```
api/v1alpha2/           # CRD type definitions
internal/controller/    # Controller implementations
internal/clients/cf/    # Cloudflare API client
```

## Step 1: Define API Types

Create `api/v1alpha2/<resource>_types.go`:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=<shortname>
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type MyResource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   MyResourceSpec   `json:"spec,omitempty"`
    Status MyResourceStatus `json:"status,omitempty"`
}

type MyResourceSpec struct {
    // Cloudflare API credentials
    Cloudflare CloudflareDetails `json:"cloudflare,omitempty"`

    // Resource-specific fields
    Name    string `json:"name,omitempty"`
    Comment string `json:"comment,omitempty"`
}

type MyResourceStatus struct {
    // Cloudflare resource ID
    ResourceID string `json:"resourceId,omitempty"`

    // Account ID for validation
    AccountID string `json:"accountId,omitempty"`

    // Current state
    State string `json:"state,omitempty"`

    // ObservedGeneration for drift detection
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Conditions for status reporting
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

## Step 2: Implement Controller

Create `internal/controller/<resource>/controller.go`:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch resource
    obj := &v1alpha2.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // 2. Initialize API client (use OperatorNamespace for cluster-scoped)
    api, err := cf.NewAPIClientFromDetails(r.ctx, r.Client,
        controller.OperatorNamespace, obj.Spec.Cloudflare)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 3. Handle deletion
    if obj.GetDeletionTimestamp() != nil {
        return r.handleDeletion()
    }

    // 4. Add finalizer
    if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
        controllerutil.AddFinalizer(obj, FinalizerName)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 5. Reconcile
    return r.reconcile()
}
```

## Required Patterns

### Status Updates (MUST use retry)

```go
err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, obj, func() {
    obj.Status.State = "active"
    controller.SetSuccessCondition(&obj.Status.Conditions, "Reconciled")
})
```

### Finalizer Operations (MUST use retry)

```go
err := controller.UpdateWithConflictRetry(ctx, r.Client, obj, func() {
    controllerutil.RemoveFinalizer(obj, FinalizerName)
})
```

### Error Handling (MUST sanitize)

```go
r.Recorder.Event(obj, corev1.EventTypeWarning, "Failed",
    cf.SanitizeErrorMessage(err))
```

### Deletion (MUST check NotFound)

```go
if err := r.cfAPI.Delete(id); err != nil {
    if !cf.IsNotFoundError(err) {
        return err
    }
    // Already deleted, continue
}
```

## Step 3: Generate Code

```bash
make manifests generate  # Generate CRD and DeepCopy
make fmt vet            # Format and check
make test               # Run tests
```

## Step 4: Register Controller

Add to `cmd/main.go`:

```go
if err = (&myresource.Reconciler{
    Client: mgr.GetClient(),
    Scheme: mgr.GetScheme(),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MyResource")
    os.Exit(1)
}
```

## Scope Rules

| Scope | Namespace Parameter | Secret Location |
|-------|---------------------|-----------------|
| Namespaced | `obj.Namespace` | Same namespace |
| Cluster | `controller.OperatorNamespace` | cloudflare-operator-system |

## Checklist

- [ ] API types with proper kubebuilder markers
- [ ] Controller with finalizer and deletion handling
- [ ] Status updates with conflict retry
- [ ] Error messages sanitized
- [ ] NotFound check on deletion
- [ ] Registered in main.go
- [ ] CRD generated with `make manifests`
- [ ] Tests pass with `make test`
