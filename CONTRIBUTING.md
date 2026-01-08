# Contributing to Cloudflare Operator / 贡献指南

Thank you for your interest in contributing to the Cloudflare Operator! This document provides guidelines and information for contributors.

感谢你对 Cloudflare Operator 的贡献兴趣！本文档为贡献者提供指南和信息。

## Table of Contents / 目录

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)

## Code of Conduct / 行为准则

Please be respectful and constructive in all interactions. We welcome contributors from all backgrounds and experience levels.

请在所有互动中保持尊重和建设性。我们欢迎来自各种背景和经验水平的贡献者。

## Getting Started / 开始

### Prerequisites / 前置条件

- Go 1.24+
- Docker
- kubectl
- A Kubernetes cluster (minikube, kind, or remote cluster)
- Cloudflare account for testing

### Fork and Clone / Fork 并克隆

```bash
# Fork the repository on GitHub, then:
git clone https://github.com/<your-username>/cloudflare-operator.git
cd cloudflare-operator
git remote add upstream https://github.com/StringKe/cloudflare-operator.git
```

## Development Setup / 开发环境设置

### Install Dependencies / 安装依赖

```bash
# Install Go dependencies
go mod download

# Install development tools
make tools
```

### Build / 构建

```bash
# Build the operator binary
make build

# Build Docker image
make docker-build IMG=cloudflare-operator:dev
```

### Run Locally / 本地运行

```bash
# Install CRDs to cluster
make install

# Run the operator locally (outside cluster)
make run

# Or run with delve debugger
make debug
```

## Making Changes / 修改代码

### Branch Naming / 分支命名

Use descriptive branch names:
使用描述性的分支名称：

- `feat/add-gateway-rule-support` - New features / 新功能
- `fix/tunnel-connection-retry` - Bug fixes / Bug 修复
- `docs/update-crd-reference` - Documentation / 文档
- `refactor/cleanup-controller` - Refactoring / 重构

### Adding a New CRD / 添加新 CRD

1. Define types in `api/v1alpha2/<resource>_types.go`
2. Run `make manifests generate`
3. Create controller in `internal/controller/<resource>/controller.go`
4. Add to `cmd/main.go`
5. Add tests
6. Add examples to `examples/`
7. Update documentation

### Modifying Existing CRDs / 修改现有 CRD

1. Update types in `api/v1alpha2/`
2. Run `make manifests generate`
3. Update controller logic if needed
4. Update tests
5. Update documentation

## Pull Request Process / Pull Request 流程

### Before Submitting / 提交前

```bash
# Format code
make fmt

# Run linter
make lint

# Run tests
make test

# Verify CRD generation
make manifests generate
git diff --exit-code
```

### PR Requirements / PR 要求

1. **Title**: Use [Conventional Commits](https://www.conventionalcommits.org/)
   - `feat: add support for GatewayRule`
   - `fix: resolve tunnel reconnection issue`
   - `docs: update installation guide`

2. **Description**: Include:
   - What changes were made
   - Why the changes are needed
   - How to test the changes
   - Related issues (e.g., `Fixes #123`)

3. **Tests**: Add or update tests for your changes

4. **Documentation**: Update relevant documentation

### Review Process / 审查流程

1. CI checks must pass
2. At least one maintainer approval required
3. Address all review comments
4. Squash commits before merge (if requested)

## Coding Standards / 代码规范

### Go Style / Go 风格

Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines and project conventions in `CLAUDE.md`.

遵循 [Effective Go](https://go.dev/doc/effective_go) 指南和 `CLAUDE.md` 中的项目约定。

### Key Requirements / 关键要求

```go
// ✅ Use ConflictRetry for status updates
err := controller.UpdateStatusWithConflictRetry(ctx, r.Client, obj, func() {
    obj.Status.State = "active"
})

// ✅ Use meta.SetStatusCondition for conditions
meta.SetStatusCondition(&status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    Reason:             "Reconciled",
    ObservedGeneration: obj.Generation,
})

// ✅ Sanitize error messages in events
r.Recorder.Event(obj, corev1.EventTypeWarning, "Failed",
    cf.SanitizeErrorMessage(err))

// ✅ Check NotFound before delete errors
if err := r.cfAPI.Delete(id); err != nil {
    if !cf.IsNotFoundError(err) {
        return err
    }
}
```

### Error Handling / 错误处理

- Always check and handle errors
- Use `errors.Join()` to aggregate multiple errors
- Provide context in error messages
- Never expose sensitive information in errors or events

## Testing / 测试

### Unit Tests / 单元测试

```bash
# Run all tests
make test

# Run specific test
go test ./internal/controller/... -run TestTunnelReconciler

# Run with coverage
make test-coverage
```

### E2E Tests / E2E 测试

```bash
# Run E2E tests (requires kind cluster)
make test-e2e
```

### Writing Tests / 编写测试

```go
var _ = Describe("TunnelController", func() {
    Context("When creating a Tunnel", func() {
        It("Should create cloudflared deployment", func() {
            // Test implementation
        })
    })
})
```

## Documentation / 文档

### Where to Update / 更新位置

| Change Type | Files to Update |
|-------------|-----------------|
| CRD changes | `docs/en/api-reference/`, `docs/zh/api-reference/` |
| New feature | `README.md`, `README_zh.md`, `docs/*/` |
| Examples | `examples/`, `config/samples/` |
| API changes | `CHANGELOG.md` |

### Documentation Style / 文档风格

- Use bilingual format (English and Chinese) where applicable
- Include code examples
- Keep examples up-to-date with actual CRD specs

## Getting Help / 获取帮助

- Open an issue for bugs or feature requests
- Join discussions in GitHub Discussions
- Tag maintainers in PR comments if needed

---

Thank you for contributing! / 感谢你的贡献！
