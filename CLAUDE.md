# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

**Cloudflare Operator** - Kubernetes Operator for managing Cloudflare Tunnels and DNS records.

**Fork 说明**: 本项目 fork 自 [adyanth/cloudflare-operator](https://github.com/adyanth/cloudflare-operator)，目标是扩展为完整的 **Cloudflare Zero Trust Kubernetes Operator**。

**技术栈**：
- Go 1.24
- Kubebuilder v4
- controller-runtime v0.20
- cloudflare-go SDK

## 当前实现状态

### API Group (当前)
- **Group**: `networking.cloudflare-operator.io`
- **版本**: v1alpha1 (deprecated), v1alpha2 (storage version)

### 已实现的 CRD (4个)

| CRD | 作用域 | 说明 |
|-----|--------|------|
| `Tunnel` | Namespaced | 管理 Cloudflare Tunnel，支持创建新 Tunnel 或使用已有 Tunnel |
| `ClusterTunnel` | Cluster | 集群级别的 Tunnel，可被多个 namespace 共享 |
| `TunnelBinding` | Namespaced | 将 K8s Service 绑定到 Tunnel，暴露服务并管理 DNS |
| `AccessTunnel` | Namespaced | 创建访问 Cloudflare Access 保护服务的客户端 |

### 已实现的控制器

```
internal/controller/
├── tunnel_controller.go        # Tunnel reconciler
├── clustertunnel_controller.go # ClusterTunnel reconciler
├── tunnelbinding_controller.go # TunnelBinding reconciler
└── (accesstunnel 未完全实现)
```

### 当前工作流程

```
Tunnel/ClusterTunnel
       │
       ├── 创建/使用 Cloudflare Tunnel
       ├── 生成 ConfigMap (cloudflared 配置)
       ├── 生成 Secret (tunnel credentials)
       └── 部署 Deployment (cloudflared pod)
              │
              ▼
       TunnelBinding
              │
              ├── 监听 Service
              ├── 更新 ConfigMap (添加 ingress 规则)
              └── 创建 DNS CNAME 记录
```

## 未来设计目标

详细设计请参考: [docs/design/ZERO_TRUST_OPERATOR_DESIGN.md](docs/design/ZERO_TRUST_OPERATOR_DESIGN.md)

### 目标 API Group
- **Group**: `cloudflare.com`
- **Annotation 前缀**: `cloudflare.com/`

### 计划实现的 CRD (21个)

#### 网络层 (Networks)
- `Tunnel` / `ClusterTunnel` - 现有，需迁移 API Group
- `NetworkRoute` - CIDR → Tunnel 路由映射 (私有网络访问核心)
- `VirtualNetwork` - 网络隔离，多租户支持
- `WARPConnector` - Site-to-Site 连接

#### 服务层 (Services)
- `TunnelBinding` - 公开访问 (现有)
- `PrivateService` - 私有访问，仅 WARP 可达

#### 设备层 (Devices)
- `DevicePostureRule` - 设备安全检查规则
- `DevicePostureIntegration` - 第三方集成
- `DeviceSettingsPolicy` - Split Tunnels / Local Domain Fallback

#### 网关层 (Gateway)
- `GatewayRule` - DNS/HTTP/L4/Egress 过滤规则
- `GatewayList` - IP/域名/URL 列表
- `GatewayLocation` - DNS over HTTPS 位置
- `GatewayConfiguration` - 全局网关配置

#### 身份层 (Access)
- `AccessApplication` - Access 应用
- `AccessGroup` - 访问组
- `AccessPolicy` - 访问策略
- `AccessServiceToken` - 服务令牌
- `AccessIdentityProvider` - IdP 配置

### 两种访问模式

```
私有访问 (WARP Only)                    公开访问 (Access Protected)
┌─────────────────────┐                ┌─────────────────────┐
│ WARP Client         │                │ Browser             │
│ ↓                   │                │ ↓                   │
│ 设备注册 + 态势检查   │                │ 公开域名             │
│ ↓                   │                │ ↓                   │
│ 私有 IP / 内部域名    │                │ Access 登录          │
│ 10.244.x.x          │                │ ↓                   │
│ ↓                   │                │ Cloudflare Edge     │
│ Cloudflare Edge     │                │ ↓                   │
│ ↓                   │                │ Tunnel              │
│ Tunnel (WARP Route) │                │ ↓                   │
│ ↓                   │                │ K8s Service         │
│ K8s Service         │                └─────────────────────┘
└─────────────────────┘
```

## 常用命令

### 开发与测试
```bash
make manifests          # 生成 CRD 和 Webhook 配置
make generate           # 生成 DeepCopy 方法
make fmt                # 格式化代码
make vet                # 运行 Go vet
make test               # 运行单元测试
make test-e2e           # 在 Kind 集群上运行 E2E 测试
make lint               # 运行 golangci-lint
make lint-fix           # 自动修复 lint 问题
```

### 构建与运行
```bash
make build              # 构建二进制文件到 bin/manager
make run                # 本地运行 Operator
make docker-build       # 构建 Docker 镜像
make docker-buildx      # 多平台构建
```

### 部署
```bash
make install            # 安装 CRD 到集群
make uninstall          # 卸载 CRD
make deploy             # 部署 Operator 到集群
make undeploy           # 移除 Operator
```

## 代码结构

```
api/
├── v1alpha1/                 # 旧版 API (deprecated)
│   ├── tunnel_types.go       # Tunnel CRD
│   ├── clustertunnel_types.go
│   ├── tunnelbinding_types.go
│   └── accesstunnel_types.go
└── v1alpha2/                 # 当前存储版本
    ├── tunnel_types.go       # 支持 deployPatch
    └── clustertunnel_types.go

internal/
├── controller/
│   ├── tunnel_controller.go
│   ├── clustertunnel_controller.go
│   ├── tunnelbinding_controller.go
│   ├── tunnel.go             # Tunnel 接口定义
│   ├── tunnel_adapter.go     # Tunnel/ClusterTunnel 适配器
│   └── generic_tunnel_reconciler.go  # 共享 reconcile 逻辑
├── clients/
│   ├── cf/                   # Cloudflare API 客户端
│   │   ├── api.go
│   │   └── configuration.go
│   └── k8s/
│       └── kubectl_apply.go
└── webhook/                  # 验证 Webhooks

config/                       # Kustomize 部署清单
docs/
├── design/                   # 设计文档
│   └── ZERO_TRUST_OPERATOR_DESIGN.md
└── migration/                # 迁移指南
```

## 关键代码模式

### Tunnel 接口模式
`Tunnel` 和 `ClusterTunnel` 通过 `Tunnel` 接口统一:

```go
// internal/controller/tunnel.go
type Tunnel interface {
    GetObject() client.Object
    GetNamespace() string
    GetName() string
    GetSpec() networkingv1alpha2.TunnelSpec
    GetStatus() networkingv1alpha2.TunnelStatus
    SetStatus(networkingv1alpha2.TunnelStatus)
    DeepCopyTunnel() Tunnel
}
```

### GenericTunnelReconciler
通用 reconciler 接口，`TunnelReconciler` 和 `ClusterTunnelReconciler` 都实现此接口:

```go
// internal/controller/generic_tunnel_reconciler.go
type GenericTunnelReconciler interface {
    GetClient() client.Client
    GetRecorder() record.EventRecorder
    GetScheme() *runtime.Scheme
    GetContext() context.Context
    GetLog() logr.Logger
    GetTunnel() Tunnel
    GetCfAPI() *cf.API
    SetCfAPI(*cf.API)
    // ...
}
```

## 测试

- **框架**: Ginkgo v2 + Gomega
- **K8s 测试版本**: 1.31.0 (envtest)
- **单元测试**: `internal/controller/*_test.go`
- **E2E 测试**: `test/e2e/e2e_test.go`

```bash
# 运行特定测试
go test -v ./internal/controller/... -run TestXxx

# 运行带覆盖率的测试
make test
```

## 代码规范

- 使用 golangci-lint v2.1.5
- 修改 CRD 后必须运行 `make manifests generate`
- 遵循 Conventional Commits 规范

## 核心依赖

- `github.com/cloudflare/cloudflare-go` - Cloudflare API 客户端
- `sigs.k8s.io/controller-runtime` - Kubernetes Operator 框架
- `k8s.io/api`, `k8s.io/client-go` - Kubernetes API

## 上游 PR 参考

待合并的功能 PR (来自 adyanth/cloudflare-operator):
- PR #115: Access Config 支持
- PR #166: FQDN 变更时 DNS 清理
- PR #158: Tunnel Secret Finalizer
- PR #140: Dummy TunnelBinding (无 Service 的 binding)
- PR #178: Leader Election 修复

## 参考资源

- [Cloudflare Zero Trust Docs](https://developers.cloudflare.com/cloudflare-one/)
- [Cloudflare API Reference](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
- [原始仓库](https://github.com/adyanth/cloudflare-operator)
