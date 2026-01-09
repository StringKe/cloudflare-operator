---
name: code-review
description: Review code changes for cloudflare-operator project. Checks for code quality standards, security issues, and best practices. Use when reviewing PRs, checking code before commit, or validating changes.
allowed-tools: Read, Grep, Glob, Bash
user-invocable: true
---

# Code Review Standards

## Overview

Review code changes against cloudflare-operator project standards. This skill checks for common issues and ensures compliance with project conventions.

## Quick Review Commands

```bash
# Run all checks
make fmt vet test lint

# Check specific file
go vet ./path/to/file.go
```

## Critical Checks (P0 - Must Fix)

### 1. Status Update Without Retry

**BAD:**
```go
obj.Status.State = "active"
r.Status().Update(ctx, obj)
```

**GOOD:**
```go
controller.UpdateStatusWithConflictRetry(ctx, r.Client, obj, func() {
    obj.Status.State = "active"
})
```

### 2. Finalizer Without Retry

**BAD:**
```go
controllerutil.RemoveFinalizer(obj, FinalizerName)
r.Update(ctx, obj)
```

**GOOD:**
```go
controller.UpdateWithConflictRetry(ctx, r.Client, obj, func() {
    controllerutil.RemoveFinalizer(obj, FinalizerName)
})
```

### 3. Sensitive Data in Events

**BAD:**
```go
r.Recorder.Event(obj, "Warning", "Failed", err.Error())
```

**GOOD:**
```go
r.Recorder.Event(obj, "Warning", "Failed", cf.SanitizeErrorMessage(err))
```

### 4. Missing NotFound Check on Delete

**BAD:**
```go
if err := r.cfAPI.Delete(id); err != nil {
    return err
}
```

**GOOD:**
```go
if err := r.cfAPI.Delete(id); err != nil {
    if !cf.IsNotFoundError(err) {
        return err
    }
    // Already deleted
}
```

### 5. Empty Namespace for Cluster-Scoped Resources

**BAD:**
```go
cf.NewAPIClientFromDetails(ctx, r.Client, "", obj.Spec.Cloudflare)
```

**GOOD:**
```go
cf.NewAPIClientFromDetails(ctx, r.Client, controller.OperatorNamespace, obj.Spec.Cloudflare)
```

## Important Checks (P1 - Should Fix)

### 6. Condition Management

**BAD:**
```go
obj.Status.Conditions = append(obj.Status.Conditions, condition)
```

**GOOD:**
```go
meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    Reason:             "Reconciled",
    ObservedGeneration: obj.Generation,
})
```

### 7. Missing Watch for Dependencies

If resource references other resources (Tunnel, VirtualNetwork), must add Watch:

```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha2.MyResource{}).
        Watches(&v1alpha2.Tunnel{},
            handler.EnqueueRequestsFromMapFunc(r.findResourcesForTunnel)).
        Complete(r)
}
```

### 8. Error Aggregation on Delete

When deleting multiple items, aggregate errors:

```go
var errs []error
for _, item := range items {
    if err := delete(item); err != nil {
        errs = append(errs, err)
    }
}
if len(errs) > 0 {
    return errors.Join(errs...)  // Don't remove finalizer
}
// All success, remove finalizer
```

## Review Checklist

### Controller Logic
- [ ] Finalizer added before any Cloudflare operations
- [ ] Finalizer removed only after successful cleanup
- [ ] Status updates use conflict retry
- [ ] Deletion checks NotFound error
- [ ] Error messages sanitized

### API Types
- [ ] Proper kubebuilder markers
- [ ] Status has ObservedGeneration
- [ ] Status has Conditions slice
- [ ] Scope correctly set (Cluster vs Namespaced)

### Security
- [ ] No hardcoded credentials
- [ ] Secrets accessed via K8s Secret API
- [ ] RBAC permissions minimal

### Testing
- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] No new lint warnings

## Running Review

```bash
# Check for common issues
grep -r "r.Status().Update" internal/controller/
grep -r "r.Update(ctx" internal/controller/ | grep -v "UpdateWithConflictRetry"
grep -r 'err.Error()' internal/controller/ | grep -i event
grep -r 'NewAPIClientFromDetails.*""' internal/controller/

# Run full validation
make fmt vet test lint build
```

## Output Format

When reviewing, report issues as:

```
## Review Results

### P0 - Critical (Must Fix)
- [file:line] Issue description

### P1 - Important (Should Fix)
- [file:line] Issue description

### Suggestions
- [file:line] Suggestion

### Summary
- Total issues: X (P0: Y, P1: Z)
- Tests: PASS/FAIL
- Lint: PASS/FAIL
```
