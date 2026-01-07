# Cloudflare Zero Trust Kubernetes Operator - Implementation Plan

## Executive Summary

本实现计划将现有的 cloudflare-operator (fork 自 adyanth/cloudflare-operator) 转型为完整的 Cloudflare Zero Trust Kubernetes Operator。计划分为 6 个阶段，涵盖：

- API Group 从 `networking.cloudflare-operator.io` 迁移到 `cloudflare.com`
- 实现 21 个 CRD，覆盖 Networks、Devices、Gateway 和 Access 层
- 集成 cloudflare-go SDK v0.115.0+ Zero Trust APIs
- 完整的测试和文档

---

## Phase 0: 基础和 Bug 修复

### 0.1 目标

- 合并关键的上游 bug 修复
- 建立新 API Group 基础设施
- 设置向后兼容层

### 0.2 上游 PR 集成

| PR | 描述 | 优先级 | 风险 |
|----|------|--------|------|
| #178 | Leader Election 修复 | 关键 | 低 |
| #166 | FQDN 变更 DNS 清理 | 高 | 中 |
| #158 | Tunnel Secret Finalizer | 高 | 低 |
| #140 | Dummy TunnelBinding 支持 | 中 | 低 |
| #115 | Access Config 支持 | 高 | 中 |

**实现步骤:**

1. 从 main 创建 feature 分支
2. Cherry-pick 或手动合并每个 PR
3. 解决与当前 v1alpha2 实现的冲突
4. 运行现有测试套件
5. 手动测试验证

### 0.3 API Group 迁移基础设施

**当前状态:**
- API Group: `networking.cloudflare-operator.io`
- 存储版本: v1alpha1 (deprecated), v1alpha2 (current)
- Labels/Annotations 前缀: `cloudflare-operator.io/`

**目标状态:**
- API Group: `cloudflare.com`
- 存储版本: v1alpha1 (new, under `cloudflare.com`)
- Labels/Annotations 前缀: `cloudflare.com/`

**迁移策略:**

```
Phase 0.3a: 创建新 API Group 结构
├── api/
│   ├── v1alpha1/        # 现有 (networking.cloudflare-operator.io)
│   ├── v1alpha2/        # 现有 hub (networking.cloudflare-operator.io)
│   └── cloudflare/
│       └── v1alpha1/    # 新 API Group (cloudflare.com)
```

**需创建的关键文件:**

1. `api/cloudflare/v1alpha1/groupversion_info.go`
   - 定义新 API Group `cloudflare.com`
   - 注册 SchemeBuilder

2. `api/cloudflare/v1alpha1/common_types.go`
   - 共享类型: CloudflareRef, SecretRef, ConditionType
   - 标准状态条件

3. 更新 `PROJECT` 文件
   - 添加新域名: `cloudflare.com`
   - 注册新资源

### 0.4 向后兼容层

**方法: 双 API 支持**

Operator 在过渡期间同时支持两个 API Group：

```go
// cmd/main.go additions
import (
    cfv1alpha1 "github.com/adyanth/cloudflare-operator/api/cloudflare/v1alpha1"
)

func init() {
    utilruntime.Must(cfv1alpha1.AddToScheme(scheme))
}
```

**弃用时间线:**
- Phase 0-2: 两个 API 完全支持
- Phase 3-4: 旧 API 弃用，conversion webhooks 激活
- Phase 5+: 旧 API 移除 (主版本升级)

### 0.5 交付物

- [ ] 所有上游 PR 合并并测试
- [ ] 新 `cloudflare.com` API Group 脚手架
- [ ] main.go 中双 API 注册
- [ ] 更新的 RBAC 清单
- [ ] 迁移文档草案

---

## Phase 1: 私有访问核心

### 1.1 目标

实现 Zero Trust Network Access (ZTNA) 核心基础设施，使私有服务只能通过 WARP 客户端访问。

### 1.2 CRD 实现顺序

```
Tunnel 增强 + VirtualNetwork
NetworkRoute
PrivateService + DeviceSettingsPolicy
```

### 1.3 Tunnel/ClusterTunnel 增强

**需要的变更:**

```go
// api/cloudflare/v1alpha1/tunnel_types.go
type TunnelSpec struct {
    // 现有字段...

    // NEW: WARP Routing 配置
    WARPRouting *WARPRoutingSpec `json:"warpRouting,omitempty"`

    // NEW: Virtual Network 引用
    VirtualNetworkRef *VirtualNetworkRef `json:"virtualNetworkRef,omitempty"`
}

type WARPRoutingSpec struct {
    // 通过此 tunnel 启用私有网络路由
    Enabled bool `json:"enabled"`
}

type VirtualNetworkRef struct {
    Name string `json:"name"`
}
```

**Controller 变更:**

更新 `internal/controller/generic_tunnel_reconciler.go`:

1. 将 WARP routing 配置添加到 ConfigMap 生成
2. 调用 Cloudflare API 更新 tunnel 配置并启用 warp-routing

**Cloudflare API 集成:**

```go
// internal/clients/cf/tunnel.go (new)
func (c *API) UpdateTunnelConfiguration(tunnelID string, config TunnelConfiguration) error {
    ctx := context.Background()
    rc := cloudflare.AccountIdentifier(c.ValidAccountId)
    params := cloudflare.TunnelConfigurationParams{
        TunnelID: tunnelID,
        Config: cloudflare.TunnelConfiguration{
            WarpRouting: &cloudflare.WarpRoutingConfig{
                Enabled: config.WarpRouting.Enabled,
            },
            Ingress: config.Ingress,
        },
    }
    _, err := c.CloudflareClient.UpdateTunnelConfiguration(ctx, rc, params)
    return err
}
```

### 1.4 VirtualNetwork CRD

**目的:** 定义隔离的虚拟网络，用于多租户和环境分离。

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: VirtualNetwork
metadata:
  name: production-vnet
spec:
  name: "Production Network"
  comment: "Production environment isolation"
  isDefault: false
  cloudflareRef:
    accountId: ""
    secretRef:
      name: "cloudflare-credentials"
      namespace: "cloudflare-system"
status:
  virtualNetworkId: ""
  isDefault: false
  ready: false
  conditions: []
```

### 1.5 NetworkRoute CRD

**目的:** 定义 CIDR 到 Tunnel 的路由，用于私有网络访问。

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: NetworkRoute
metadata:
  name: k8s-pod-network
  namespace: cloudflare-system
spec:
  network: "10.244.0.0/16"
  comment: "Kubernetes Pod Network"
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"
  virtualNetworkRef:
    name: "production-vnet"
  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"
status:
  routeId: ""
  tunnelId: ""
  virtualNetworkId: ""
  ready: false
  conditions: []
```

**Controller 流程:**

```
1. 验证 CIDR 格式
2. 解析 TunnelRef 获取 TunnelID
3. 解析 VirtualNetworkRef (可选)
4. 通过 Cloudflare API 创建/更新路由
5. 添加 finalizer 用于清理
6. 更新状态
```

### 1.6 PrivateService CRD

**目的:** 高级抽象，用于通过私有网络路由暴露 K8s Service。

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: PrivateService
metadata:
  name: internal-api
  namespace: production
spec:
  serviceRef:
    name: "api-server"
    port: 8080
  privateAccess:
    exposeClusterIP: true
    internalDNS:
      enabled: true
      hostname: "api.internal.corp.com"
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"
  autoCreateRoute:
    enabled: true
    virtualNetworkRef:
      name: "production-vnet"
status:
  clusterIP: ""
  internalHostname: ""
  routeId: ""
  ready: false
  conditions: []
```

### 1.7 DeviceSettingsPolicy CRD

**目的:** 配置 WARP 客户端行为，包括 Split Tunnels。

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DeviceSettingsPolicy
metadata:
  name: default-policy
  namespace: cloudflare-system
spec:
  name: "Default Device Policy"
  enabled: true
  precedence: 100
  default: false
  match: 'identity.email matches ".*@example.com"'
  splitTunnelInclude:
    - address: "10.0.0.0/8"
      description: "Internal networks"
    - address: "10.244.0.0/16"
      description: "K8s Pod network"
  localDomainFallback:
    - suffix: "internal.corp.com"
      dnsServers: ["10.0.0.53"]
  warpSettings:
    tunnelProtocol: "wireguard"
    autoConnect: 0
  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"
status:
  policyId: ""
  ready: false
  conditions: []
```

### 1.8 Phase 1 交付物

- [ ] 增强的 Tunnel/ClusterTunnel 支持 WARP routing
- [ ] VirtualNetwork CRD 和 controller
- [ ] NetworkRoute CRD 和 controller
- [ ] PrivateService CRD 和 controller
- [ ] DeviceSettingsPolicy CRD 和 controller
- [ ] 所有新 controller 的单元测试
- [ ] 与 mock Cloudflare API 的集成测试

---

## Phase 2: 公开访问和网关

### 2.1 目标

实现 Access 保护的公开暴露和用于流量过滤的 Gateway 规则。

### 2.2 CRD 实现顺序

```
TunnelBinding 增强 + AccessApplication
AccessGroup + AccessPolicy
GatewayRule + GatewayList
```

### 2.3 TunnelBinding 增强

**需要的增强:**

```go
type TunnelBindingSpec struct {
    Subjects  []TunnelBindingSubject `json:"subjects"`
    TunnelRef TunnelRef              `json:"tunnelRef"`

    // NEW: 公开端点的 Access 保护
    AccessConfig *AccessConfig `json:"accessConfig,omitempty"`

    // NEW: 引用现有 AccessApplication
    AccessApplicationRef *AccessApplicationRef `json:"accessApplicationRef,omitempty"`
}

type AccessConfig struct {
    Enabled bool `json:"enabled"`
    Type    string `json:"type"` // self_hosted, saas, bookmark

    Authentication *AuthenticationConfig `json:"authentication,omitempty"`
    Policies       []InlinePolicy        `json:"policies,omitempty"`
    Cookies        *CookieConfig         `json:"cookies,omitempty"`
    Appearance     *AppearanceConfig     `json:"appearance,omitempty"`
}
```

### 2.4 AccessApplication CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessApplication
metadata:
  name: admin-portal
  namespace: production
spec:
  name: "Admin Portal"
  domain: "admin.example.com"
  type: self_hosted
  sessionDuration: "24h"
  allowedIdps: []
  autoRedirectToIdentity: false
  appLauncherVisible: true

  policies:
    - name: "allow-admins"
      decision: allow
      precedence: 1
      include:
        - group:
            id: ""
        - emailDomain:
            domain: "example.com"
      require:
        - devicePosture:
            integrationId: ""

  policyRefs:
    - name: "admin-policy"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  applicationId: ""
  aud: ""
  ready: false
  conditions: []
```

### 2.5 AccessGroup CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessGroup
metadata:
  name: developers
  namespace: cloudflare-system
spec:
  name: "Developers"

  include:
    - emailDomain:
        domain: "example.com"
    - email:
        email: "contractor@external.com"
    - okta:
        name: "developers"
        identityProviderId: ""
    - azureAD:
        id: "group-uuid"
        identityProviderId: ""

  require:
    - warp: true
    - certificate: true

  exclude:
    - email:
        email: "blocked@example.com"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  groupId: ""
  ready: false
  conditions: []
```

### 2.6 GatewayRule CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: GatewayRule
metadata:
  name: allow-internal-access
  namespace: cloudflare-system
spec:
  name: "Allow Internal Network Access"
  enabled: true
  precedence: 100

  # 规则类型: dns, http, l4, egress
  filters:
    - l4

  # Wirefilter 表达式
  traffic: 'net.dst.ip in {10.244.0.0/16 10.96.0.0/12}'
  identity: 'identity.groups.name[*] in {"developers", "sre"}'
  devicePosture: 'any(device_posture.checks.passed[*] in {"disk_encryption"})'

  action: allow

  ruleSettings:
    blockPageEnabled: false

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  ruleId: ""
  ready: false
  conditions: []
```

### 2.7 GatewayList CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: GatewayList
metadata:
  name: internal-services
  namespace: cloudflare-system
spec:
  name: "Internal Services"
  type: IP  # SERIAL, URL, DOMAIN, EMAIL, IP, HOSTNAME

  # 静态项目
  items:
    - value: "10.244.0.0/16"
      description: "Pod network"

  # 从 ConfigMap 动态获取
  itemsFrom:
    - configMapRef:
        name: "internal-ips"
        key: "ips.txt"

  # 从 Services 自动同步
  serviceSelector:
    matchLabels:
      expose: "private"
    namespaces:
      - production

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  listId: ""
  itemCount: 0
  ready: false
  conditions: []
```

### 2.8 Phase 2 交付物

- [ ] 增强的 TunnelBinding 支持 Access 集成
- [ ] AccessApplication CRD 和 controller
- [ ] AccessGroup CRD 和 controller
- [ ] AccessPolicy CRD 和 controller
- [ ] GatewayRule CRD 和 controller
- [ ] GatewayList CRD 和 controller
- [ ] 单元和集成测试
- [ ] 示例配置

---

## Phase 3: 身份集成

### 3.1 目标

实现身份提供者管理、服务令牌和设备态势集成。

### 3.2 CRD 实现顺序

```
AccessIdentityProvider + AccessServiceToken
DevicePostureRule + DevicePostureIntegration
```

### 3.3 AccessIdentityProvider CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessIdentityProvider
metadata:
  name: okta-sso
spec:
  name: "Okta SSO"
  type: okta  # azuread, okta, google, oidc, saml, github, etc.

  config:
    oktaAccount: "example.okta.com"
    clientId: "xxx"
    clientSecretRef:
      name: "okta-credentials"
      key: "client-secret"

  scimConfig:
    enabled: true
    secretRef:
      name: "scim-token"
      key: "token"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  providerId: ""
  ready: false
  conditions: []
```

### 3.4 AccessServiceToken CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: AccessServiceToken
metadata:
  name: ci-cd-token
  namespace: production
spec:
  name: "CI/CD Pipeline Token"
  duration: "8760h"  # 1 year

  tokenSecretRef:
    name: "ci-cd-access-token"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  tokenId: ""
  clientId: ""
  expiresAt: ""
  ready: false
  conditions: []
```

### 3.5 DevicePostureRule CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DevicePostureRule
metadata:
  name: disk-encryption
  namespace: cloudflare-system
spec:
  name: "Disk Encryption Required"
  type: disk_encryption  # firewall, os_version, warp, file, etc.

  match:
    - platform: windows
    - platform: mac
    - platform: linux

  input:
    requireAll: true

  schedule: "1h"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  ruleId: ""
  ready: false
  conditions: []
```

### 3.6 DevicePostureIntegration CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DevicePostureIntegration
metadata:
  name: crowdstrike
spec:
  name: "CrowdStrike Integration"
  type: crowdstrike_s2s

  config:
    clientIdRef:
      name: "crowdstrike-credentials"
      key: "client-id"
    clientSecretRef:
      name: "crowdstrike-credentials"
      key: "client-secret"
    apiUrl: "https://api.crowdstrike.com"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  integrationId: ""
  ready: false
  conditions: []
```

### 3.7 Phase 3 交付物

- [ ] AccessIdentityProvider CRD 和 controller
- [ ] AccessServiceToken CRD 和 controller
- [ ] DevicePostureRule CRD 和 controller
- [ ] DevicePostureIntegration CRD 和 controller
- [ ] 服务令牌的 Secret 生成
- [ ] 单元和集成测试

---

## Phase 4: 高级功能

### 4.1 目标

实现 DNS 管理、WARP Connector 和 Gateway 高级功能。

### 4.2 CRD 实现顺序

```
DNSRecord + GatewayLocation + GatewayConfiguration
WARPConnector + AccessCertificate
```

### 4.3 DNSRecord CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: DNSRecord
metadata:
  name: api-record
  namespace: production
  annotations:
    external-dns.alpha.kubernetes.io/hostname: "api.example.com"
spec:
  type: CNAME
  name: "api.example.com"

  content: "xxx.cloudflare-operator.io"
  # OR
  tunnelRef:
    kind: ClusterTunnel
    name: "main-tunnel"

  ttl: 1  # auto
  proxied: true

  zoneSelector:
    zoneName: "example.com"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  recordId: ""
  zoneId: ""
  ready: false
  conditions: []
```

### 4.4 GatewayConfiguration CRD (Cluster-scoped)

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: GatewayConfiguration
metadata:
  name: default
spec:
  settings:
    activityLog:
      enabled: true
    tlsDecrypt:
      enabled: true
    antiVirus:
      enabled: true
      notificationSettings:
        enabled: true
        supportUrl: "https://support.example.com"
    blockPage:
      enabled: true
      headerText: "Access Blocked"
      footerText: "Contact IT for assistance"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"
      namespace: "cloudflare-system"

status:
  ready: false
  conditions: []
```

### 4.5 WARPConnector CRD

```yaml
apiVersion: cloudflare.com/v1alpha1
kind: WARPConnector
metadata:
  name: datacenter-connector
  namespace: cloudflare-system
spec:
  name: "Datacenter to Cloud Connector"

  tunnelRef:
    kind: ClusterTunnel
    name: "warp-connector-tunnel"

  siteConfig:
    localNetworks:
      - "192.168.0.0/16"
    remoteNetworks:
      - "10.0.0.0/8"

  cloudflareRef:
    secretRef:
      name: "cloudflare-credentials"

status:
  connectorId: ""
  ready: false
  conditions: []
```

### 4.6 Phase 4 交付物

- [ ] DNSRecord CRD 和 controller
- [ ] GatewayLocation CRD 和 controller
- [ ] GatewayConfiguration CRD 和 controller
- [ ] WARPConnector CRD 和 controller
- [ ] AccessCertificate CRD 和 controller
- [ ] external-dns annotation 兼容性
- [ ] 单元和集成测试

---

## Phase 5: 文档和测试

### 5.1 测试策略

**单元测试:**

每个 controller 需要:
- Reconciliation 逻辑测试
- Status 更新测试
- Finalizer 测试
- 错误处理测试

**集成测试:**

```go
// test/integration/suite_test.go
var _ = BeforeSuite(func() {
    // Start envtest
    // Install CRDs
    // Start controllers
})

var _ = Describe("VirtualNetwork Controller", func() {
    It("should create virtual network on Cloudflare", func() {
        // Mock Cloudflare API
        // Create VirtualNetwork CR
        // Verify API called
        // Verify status updated
    })
})
```

**E2E 测试:**

```go
// test/e2e/zero_trust_test.go
var _ = Describe("Zero Trust E2E", func() {
    It("should enable private access flow", func() {
        // Create ClusterTunnel with warpRouting
        // Create VirtualNetwork
        // Create NetworkRoute
        // Create PrivateService
        // Verify end-to-end connectivity
    })
})
```

### 5.2 文档结构

```
docs/
├── getting-started.md
├── configuration/
│   ├── tunnel-and-cluster-tunnel.md
│   ├── network-route.md
│   ├── virtual-network.md
│   ├── private-service.md
│   ├── device-settings-policy.md
│   ├── gateway-rule.md
│   ├── gateway-list.md
│   ├── access-application.md
│   ├── access-group.md
│   ├── access-identity-provider.md
│   └── dns-record.md
├── examples/
│   ├── private-access/
│   ├── public-access/
│   ├── gateway-policies/
│   └── multi-tenant/
├── migrations/
│   └── api-group-migration.md
└── design/
    ├── ZERO_TRUST_OPERATOR_DESIGN.md
    └── IMPLEMENTATION_PLAN.md
```

### 5.3 Phase 5 交付物

- [ ] 完整的单元测试覆盖 (>80%)
- [ ] 集成测试套件
- [ ] E2E 测试套件
- [ ] API 文档
- [ ] 用户指南
- [ ] 迁移指南
- [ ] 示例配置

---

## Phase 6: 发布准备

### 6.1 Helm Chart 更新

更新 Helm chart 以包含:
- 所有新 CRD
- 更新的 RBAC 权限
- 新功能的配置选项
- 迁移帮助工具

### 6.2 RBAC 权限

```yaml
# config/rbac/role.yaml additions
rules:
  # 现有权限...

  # Zero Trust Networks
  - apiGroups: ["cloudflare.com"]
    resources: ["virtualnetworks", "networkroutes", "privateservices"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Zero Trust Devices
  - apiGroups: ["cloudflare.com"]
    resources: ["deviceposturerules", "devicepostureintegrations", "devicesettingspolicies"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Zero Trust Gateway
  - apiGroups: ["cloudflare.com"]
    resources: ["gatewayrules", "gatewaylists", "gatewaylocations", "gatewayconfigurations"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Zero Trust Access
  - apiGroups: ["cloudflare.com"]
    resources: ["accessapplications", "accessgroups", "accesspolicies", "accessservicetokens", "accessidentityproviders", "accesscertificates"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # DNS
  - apiGroups: ["cloudflare.com"]
    resources: ["dnsrecords"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### 6.3 发布检查清单

- [ ] 版本升级 (0.14.0 或 1.0.0)
- [ ] Changelog
- [ ] Release notes
- [ ] Container images (multi-arch)
- [ ] Helm chart 发布
- [ ] OLM bundle 更新
- [ ] GitHub release

---

## 技术架构

### Controller 结构

```
internal/
├── controller/
│   ├── tunnel/
│   │   ├── reconciler.go           # 增强 WARP routing
│   │   └── reconciler_test.go
│   ├── clustertunnel/
│   │   ├── reconciler.go
│   │   └── reconciler_test.go
│   ├── tunnelbinding/
│   │   ├── reconciler.go           # 增强 Access config
│   │   └── reconciler_test.go
│   ├── networkroute/
│   │   ├── reconciler.go           # NEW
│   │   └── reconciler_test.go
│   ├── virtualnetwork/
│   │   ├── reconciler.go           # NEW
│   │   └── reconciler_test.go
│   ├── privateservice/
│   │   ├── reconciler.go           # NEW
│   │   └── reconciler_test.go
│   ├── device/
│   │   ├── posturerule_reconciler.go
│   │   ├── postureintegration_reconciler.go
│   │   ├── settingspolicy_reconciler.go
│   │   └── *_test.go
│   ├── gateway/
│   │   ├── rule_reconciler.go
│   │   ├── list_reconciler.go
│   │   ├── location_reconciler.go
│   │   ├── configuration_reconciler.go
│   │   └── *_test.go
│   ├── access/
│   │   ├── application_reconciler.go
│   │   ├── group_reconciler.go
│   │   ├── policy_reconciler.go
│   │   ├── servicetoken_reconciler.go
│   │   ├── identityprovider_reconciler.go
│   │   ├── certificate_reconciler.go
│   │   └── *_test.go
│   ├── dns/
│   │   ├── record_reconciler.go
│   │   └── record_reconciler_test.go
│   └── common/
│       ├── conditions.go
│       └── finalizers.go
├── clients/
│   └── cf/
│       ├── api.go                  # 现有
│       ├── configuration.go        # 现有
│       ├── tunnel.go               # NEW: Tunnel configuration API
│       ├── network.go              # NEW: Virtual networks, routes
│       ├── device.go               # NEW: Device posture, settings
│       ├── gateway.go              # NEW: Gateway rules, lists
│       ├── access.go               # NEW: Applications, groups, policies
│       └── dns.go                  # NEW: DNS records
└── webhook/
    └── cloudflare/
        └── v1alpha1/
            ├── tunnel_webhook.go
            └── validation.go
```

### Cloudflare Client 抽象

```go
// internal/clients/cf/client.go
type ZeroTrustClient struct {
    *cloudflare.API
    Log       logr.Logger
    AccountID string
}

// Network operations
func (c *ZeroTrustClient) Networks() *NetworkClient
func (c *ZeroTrustClient) Tunnels() *TunnelClient

// Device operations
func (c *ZeroTrustClient) Devices() *DeviceClient

// Gateway operations
func (c *ZeroTrustClient) Gateway() *GatewayClient

// Access operations
func (c *ZeroTrustClient) Access() *AccessClient

// DNS operations
func (c *ZeroTrustClient) DNS() *DNSClient
```

---

## 风险评估和缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| Cloudflare API 变更 | 高 | 低 | 锁定 SDK 版本，监控发布 |
| API Group 迁移破坏现有用户 | 高 | 中 | Conversion webhooks，广泛测试 |
| 多 CRD 性能问题 | 中 | 中 | 速率限制，缓存，高效 watch |
| 复杂 RBAC 需求 | 中 | 低 | 清晰文档，最小权限 |
| 测试覆盖不足 | 中 | 中 | 每个 PR 强制测试要求 |

---

## 依赖和先决条件

### 外部依赖

1. **cloudflare-go SDK v0.115.0+**
   - Zero Trust APIs
   - Network Routes
   - Virtual Networks
   - Device Posture
   - Gateway Rules
   - Access APIs

2. **Kubernetes 1.28+**
   - CRD v1 支持
   - Conversion webhooks

3. **controller-runtime v0.20+**
   - go.mod 中的当前版本

### 所需 Cloudflare API 权限

```yaml
Account Permissions:
  - Cloudflare Tunnel: Edit
  - Access: Apps and Policies: Edit
  - Access: Organizations, Identity Providers, and Groups: Edit
  - Access: Service Tokens: Edit
  - Access: Device Posture: Edit
  - Zero Trust: Edit
  - Zero Trust Gateway: Edit

Zone Permissions:
  - DNS: Edit
  - Zone: Read
```

---

## 总结

本实现计划提供了将 cloudflare-operator 转型为完整 Zero Trust Kubernetes Operator 的全面路线图。分阶段方法确保：

1. **最小中断** - 通过向后兼容性
2. **增量价值交付** - 每个阶段都有产出
3. **全面测试** - 每个阶段都有测试
4. **清晰文档** - 为用户和贡献者提供

实现覆盖 21 个 CRD，跨越 5 个功能层（Networks、Services、Devices、Gateway、Access），将此 operator 打造为 Kubernetes 上最全面的 Cloudflare Zero Trust 集成。

---

## 关键实现文件

实现此计划最关键的文件：

- `api/v1alpha2/tunnel_types.go` - 需扩展 WARP routing 和 virtual network 引用的核心类型定义
- `internal/controller/generic_tunnel_reconciler.go` - 所有新 controller 应遵循的基础 reconciler 模式
- `internal/clients/cf/api.go` - 需扩展 Zero Trust API 方法的 Cloudflare 客户端结构
- `cmd/main.go` - 所有新 reconciler 的 controller 注册点
- `docs/design/ZERO_TRUST_OPERATOR_DESIGN.md` - 详细 CRD 规格和架构参考
