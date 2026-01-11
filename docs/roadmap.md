# Cloudflare Operator 功能扩展路线图

本文档记录 cloudflare-operator 的功能扩展计划，包括新增 CRD 和现有 CRD 增强。

## 版本规划

- **v0.20.x**: CloudflareDomain 增强 + OriginCACertificate
- **v0.21.x**: R2 存储管理
- **v0.22.x**: 规则引擎 (Rulesets)
- **v0.23.x**: 域名注册管理 (Enterprise)

---

## P0: CloudflareDomain 增强

### 目标
扩展 CloudflareDomain CRD，支持完整的域名配置管理。

### 新增字段

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareDomain
metadata:
  name: example-com
spec:
  domain: example.com
  credentialsRef:
    name: cloudflare-credentials
  isDefault: false

  # ========== 新增: SSL/TLS 配置 ==========
  ssl:
    # 加密模式
    mode: full_strict  # off, flexible, full, full_strict, strict

    # TLS 版本
    minTLSVersion: "1.2"  # 1.0, 1.1, 1.2, 1.3
    tls13: enabled  # enabled, disabled

    # 自动 HTTPS 重写
    alwaysUseHttps: true
    automaticHttpsRewrites: true

    # 源服务器认证
    authenticatedOriginPull:
      enabled: false
      # 自定义证书 (可选)
      certificateSecretRef:
        name: origin-pull-cert
        namespace: cloudflare-operator-system

  # ========== 新增: 缓存配置 ==========
  cache:
    # 浏览器缓存 TTL (秒)
    browserTTL: 14400

    # 开发模式 (禁用缓存)
    developmentMode: false

    # 缓存级别
    cacheLevel: aggressive  # bypass, basic, simplified, aggressive

    # 分层缓存
    tieredCache:
      enabled: true
      topology: smart  # smart, generic

    # Cache Reserve (持久化缓存)
    cacheReserve:
      enabled: false

    # 始终在线
    alwaysOnline: true

  # ========== 新增: 安全配置 ==========
  security:
    # 安全级别
    level: medium  # off, essentially_off, low, medium, high, under_attack

    # 浏览器完整性检查
    browserCheck: true

    # Email 地址混淆
    emailObfuscation: true

    # 服务器端排除
    serverSideExclude: true

    # Hotlink 保护
    hotlinkProtection: false

    # WAF (如果启用)
    waf:
      enabled: false
      mode: block  # simulate, block, challenge

  # ========== 新增: 性能配置 ==========
  performance:
    # 压缩
    brotli: true
    gzip: true

    # HTTP 版本
    http2: true
    http3: true

    # 代码压缩
    minify:
      html: true
      css: true
      javascript: true

    # 图片优化 (需要 Pro+)
    polish: off  # off, lossless, lossy
    webp: false
    mirage: false

    # 预加载
    prefetchPreload: true

    # Early Hints
    earlyHints: true

    # Rocket Loader
    rocketLoader: false

    # 0-RTT
    zeroRTT: true

  # ========== 新增: 高级设置 ==========
  advanced:
    # 伪 IPv4
    pseudoIPv4: off  # off, add_header, overwrite_header

    # WebSocket
    websockets: true

    # 机器人攻击模式
    opportunisticEncryption: true

    # 真实 IP 头
    trueClientIPHeader: false

    # IP 地理位置
    ipGeolocation: true

status:
  # 现有状态字段
  state: Ready
  zoneID: xxx
  accountID: xxx
  nameservers:
    - ns1.cloudflare.com
    - ns2.cloudflare.com

  # 新增: 配置同步状态
  configSyncStatus:
    ssl: Synced
    cache: Synced
    security: Synced
    performance: Synced
  lastConfigSync: "2025-01-10T12:00:00Z"
```

### 实现文件
- `api/v1alpha2/cloudflareDomain_types.go` - 类型定义
- `internal/controller/cloudflareDomain/controller.go` - 控制器增强
- `internal/controller/cloudflareDomain/settings.go` - Zone 设置同步
- `internal/clients/cf/zone_settings.go` - Cloudflare API 封装

### 测试覆盖
- [ ] Zone Settings 获取/更新测试
- [ ] SSL 配置同步测试
- [ ] Cache 配置同步测试
- [ ] Security 配置同步测试
- [ ] Performance 配置同步测试

---

## P0: OriginCACertificate

### 目标
管理 Cloudflare Origin CA 证书，支持自动签发和 Kubernetes Secret 同步。

### CRD 定义

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: OriginCACertificate
metadata:
  name: origin-cert-example
  namespace: default
spec:
  # 域名引用 (用于获取凭据和 Zone ID)
  domainRef:
    name: example-com

  # 或直接指定凭据
  credentialsRef:
    name: cloudflare-credentials

  # 证书主机名
  hostnames:
    - "*.example.com"
    - "example.com"

  # 请求类型
  requestType: origin-rsa  # origin-rsa, origin-ecc

  # 有效期 (天)
  # 可选值: 7, 30, 90, 365, 730, 1095, 5475 (15年)
  validityDays: 5475

  # CSR (可选，不提供则自动生成密钥对)
  csr: ""

  # 同步到 Secret
  syncToSecret:
    enabled: true
    name: cloudflare-origin-cert
    namespace: istio-system
    # Secret 类型
    type: kubernetes.io/tls  # kubernetes.io/tls, Opaque

    # 额外标签
    labels:
      app: my-app

    # 额外注解
    annotations:
      cert-manager.io/issuer-name: cloudflare-origin-ca

  # 自动续期
  autoRenew:
    enabled: true
    renewBeforeDays: 30  # 到期前多少天续期

status:
  # 证书 ID
  certificateId: "xxx"

  # 证书序列号
  serialNumber: "xxx"

  # 证书指纹
  fingerprint: "SHA256:xxx"

  # 有效期
  notBefore: "2025-01-10T00:00:00Z"
  notAfter: "2040-01-10T00:00:00Z"

  # 主机名
  hostnames:
    - "*.example.com"
    - "example.com"

  # 证书 PEM (公钥部分)
  certificate: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----

  # Secret 引用
  secretRef:
    name: cloudflare-origin-cert
    namespace: istio-system

  # 状态
  state: Ready  # Pending, Ready, Error, Expired, Renewing

  # 条件
  conditions:
    - type: Ready
      status: "True"
      reason: CertificateIssued
      message: "Certificate successfully issued and synced to secret"
```

### 实现文件
- `api/v1alpha2/origincacertificate_types.go` - 类型定义
- `internal/controller/origincacertificate/controller.go` - 控制器
- `internal/controller/origincacertificate/secret.go` - Secret 同步
- `internal/clients/cf/origin_ca.go` - Cloudflare API 封装

### 测试覆盖
- [ ] 证书创建测试
- [ ] 证书续期测试
- [ ] Secret 同步测试
- [ ] 证书撤销测试
- [ ] 自动续期逻辑测试

---

## P1: R2Bucket

### 目标
管理 Cloudflare R2 存储桶的完整生命周期。

### CRD 定义

```yaml
apiVersion: storage.cloudflare-operator.io/v1alpha1
kind: R2Bucket
metadata:
  name: my-assets
spec:
  # 桶名称
  name: my-assets-bucket

  # 凭据引用
  credentialsRef:
    name: cloudflare-credentials

  # 区域提示 (可选)
  locationHint: APAC  # APAC, EEUR, ENAM, WEUR, WNAM

  # 存储类别
  storageClass: Standard  # Standard, InfrequentAccess

  # 司法管辖区 (可选，用于数据驻留)
  jurisdiction: ""  # eu, fedramp

  # ========== 公共访问 ==========
  publicAccess:
    # r2.dev 域名
    managedDomain:
      enabled: false

    # 自定义域名
    customDomains:
      - domain: assets.example.com
        zoneRef:
          name: example-com  # 引用 CloudflareDomain
        # 或直接指定 Zone ID
        zoneId: ""
        minTLS: "1.2"
        enabled: true

  # ========== 生命周期规则 ==========
  lifecycle:
    rules:
      - id: delete-old-logs
        enabled: true
        prefix: logs/
        expiration:
          days: 30
        # 或指定日期
        # expirationDate: "2025-12-31"

      - id: archive-backups
        enabled: true
        prefix: backups/
        transition:
          days: 90
          storageClass: InfrequentAccess

      - id: abort-multipart
        enabled: true
        abortIncompleteMultipartUpload:
          daysAfterInitiation: 7

  # ========== CORS 配置 ==========
  cors:
    rules:
      - allowedOrigins:
          - "https://example.com"
          - "https://*.example.com"
        allowedMethods:
          - GET
          - HEAD
          - PUT
        allowedHeaders:
          - "*"
        exposeHeaders:
          - ETag
        maxAgeSeconds: 3600

  # ========== 事件通知 ==========
  eventNotifications:
    - id: upload-notification
      queueRef:
        name: my-queue  # 引用 Queue CRD (未来实现)
      # 或直接指定 Queue 名称
      queueName: my-upload-queue

      # 事件类型
      actions:
        - PutObject
        - CopyObject

      # 过滤器
      prefix: uploads/
      suffix: .jpg

status:
  # 桶名称
  name: my-assets-bucket

  # 创建时间
  createdAt: "2025-01-10T12:00:00Z"

  # 区域
  location: APAC

  # 存储类别
  storageClass: Standard

  # 公共访问
  publicAccess:
    managedDomain:
      enabled: false
      url: ""
    customDomains:
      - domain: assets.example.com
        status: Active
        zoneId: xxx

  # 生命周期规则数量
  lifecycleRulesCount: 3

  # CORS 规则数量
  corsRulesCount: 1

  # 事件通知数量
  eventNotificationsCount: 1

  # 状态
  state: Ready  # Pending, Ready, Error

  # 条件
  conditions:
    - type: Ready
      status: "True"
```

### 实现文件
- `api/storage/v1alpha1/r2bucket_types.go` - 类型定义
- `internal/controller/r2bucket/controller.go` - 控制器
- `internal/controller/r2bucket/lifecycle.go` - 生命周期管理
- `internal/controller/r2bucket/cors.go` - CORS 管理
- `internal/controller/r2bucket/domain.go` - 自定义域名管理
- `internal/controller/r2bucket/notification.go` - 事件通知管理
- `internal/clients/cf/r2.go` - Cloudflare R2 API 封装

### 测试覆盖
- [ ] Bucket 创建/删除测试
- [ ] 自定义域名绑定测试
- [ ] 生命周期规则测试
- [ ] CORS 配置测试
- [ ] 事件通知测试

---

## P1: ZoneRuleset

### 目标
管理 Cloudflare Zone 级别的规则集，替代废弃的 Page Rules。

### CRD 定义

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ZoneRuleset
metadata:
  name: my-rules
spec:
  # 域名引用
  domainRef:
    name: example-com

  # 或直接指定凭据和 Zone
  credentialsRef:
    name: cloudflare-credentials
  zoneId: xxx

  # 规则集阶段
  # 参考: https://developers.cloudflare.com/ruleset-engine/reference/phases-list/
  phase: http_request_transform
  # 可用阶段:
  # - http_request_transform (URL 重写)
  # - http_request_late_transform (延迟转换)
  # - http_request_redirect (重定向)
  # - http_request_origin (源站规则)
  # - http_request_cache_settings (缓存设置)
  # - http_config_settings (配置设置)
  # - http_request_firewall_custom (自定义 WAF)
  # - http_ratelimit (速率限制)
  # - http_request_sbfm (Super Bot Fight Mode)

  # 规则定义
  rules:
    # URL 重写示例
    - name: rewrite-api-v1-to-v2
      description: "Rewrite /api/v1/* to /api/v2/*"
      enabled: true

      # 表达式 (Cloudflare Ruleset 语法)
      expression: '(http.request.uri.path matches "^/api/v1/")'

      # 动作
      action: rewrite
      actionParameters:
        uri:
          path:
            # 静态值
            value: "/api/v2/"
            # 或动态表达式
            # expression: 'regex_replace(http.request.uri.path, "^/api/v1/", "/api/v2/")'

    # Header 修改示例
    - name: add-security-headers
      description: "Add security headers to all responses"
      enabled: true
      expression: "true"
      action: rewrite
      actionParameters:
        headers:
          - operation: set
            name: X-Frame-Options
            value: DENY
          - operation: set
            name: X-Content-Type-Options
            value: nosniff
          - operation: set
            name: Strict-Transport-Security
            value: "max-age=31536000; includeSubDomains"

    # 重定向示例 (需要 phase: http_request_redirect)
    - name: redirect-old-path
      description: "Redirect old paths to new location"
      enabled: true
      expression: '(http.request.uri.path eq "/old-page")'
      action: redirect
      actionParameters:
        fromValue:
          statusCode: 301
          targetUrl:
            value: "https://example.com/new-page"

    # 缓存设置示例 (需要 phase: http_request_cache_settings)
    - name: cache-static-assets
      description: "Cache static assets for 1 day"
      enabled: true
      expression: '(http.request.uri.path.extension in {"css" "js" "png" "jpg" "gif" "svg" "woff2"})'
      action: set_cache_settings
      actionParameters:
        cache: true
        edgeTTL:
          mode: override_origin
          default: 86400
        browserTTL:
          mode: override_origin
          default: 3600

status:
  # 规则集 ID
  rulesetId: xxx

  # 规则集版本
  version: 1

  # 阶段
  phase: http_request_transform

  # 规则数量
  rulesCount: 4

  # 最后部署时间
  lastDeployed: "2025-01-10T12:00:00Z"

  # 状态
  state: Ready  # Pending, Ready, Error

  # 规则状态
  rules:
    - name: rewrite-api-v1-to-v2
      id: rule-xxx
      enabled: true
      version: 1

  # 条件
  conditions:
    - type: Ready
      status: "True"
```

### 实现文件
- `api/v1alpha2/zoneruleset_types.go` - 类型定义
- `internal/controller/zoneruleset/controller.go` - 控制器
- `internal/controller/zoneruleset/rules.go` - 规则管理
- `internal/clients/cf/rulesets.go` - Cloudflare Rulesets API 封装

### 测试覆盖
- [ ] Ruleset 创建/删除测试
- [ ] 规则 CRUD 测试
- [ ] 表达式验证测试
- [ ] 各阶段动作测试

---

## P3: DomainRegistration (Enterprise Only)

### 目标
管理 Cloudflare Registrar 域名注册。

### CRD 定义

```yaml
apiVersion: registrar.cloudflare-operator.io/v1alpha1
kind: DomainRegistration
metadata:
  name: example-com
spec:
  # 域名
  domain: example.com

  # 凭据引用
  credentialsRef:
    name: cloudflare-credentials

  # 自动续期
  autoRenew: true

  # 锁定 (防止未授权转移)
  locked: true

  # WHOIS 隐私
  privacy: true

  # 联系人信息 (可选)
  contact:
    firstName: John
    lastName: Doe
    organization: Example Inc
    email: admin@example.com
    phone: "+1.5551234567"
    address:
      street: "123 Main St"
      city: San Francisco
      state: CA
      postalCode: "94105"
      country: US

status:
  # 注册商
  registrar: cloudflare

  # 域名状态
  state: Active  # Active, Expired, Pending, TransferIn

  # 创建时间
  createdAt: "2020-01-01T00:00:00Z"

  # 过期时间
  expiresAt: "2026-01-01T00:00:00Z"

  # 自动续期
  autoRenew: true

  # 锁定状态
  locked: true

  # WHOIS 隐私
  privacy: true

  # 域名服务器
  nameservers:
    - ns1.cloudflare.com
    - ns2.cloudflare.com

  # 转入状态 (如果适用)
  transferIn:
    status: completed
    acceptFoa: completed
    approveTransfer: completed

  # 条件
  conditions:
    - type: Ready
      status: "True"
```

### 注意事项
- Registrar API 仅限 Enterprise 客户
- 需要使用 Global API Key (不支持 API Token)
- 新域名注册可能需要额外的 API 权限

---

## API 分组规划

```
networking.cloudflare-operator.io/v1alpha2
├── CloudflareDomain (增强)
├── CloudflareCredentials
├── Tunnel / ClusterTunnel
├── VirtualNetwork / NetworkRoute
├── TunnelBinding / PrivateService
├── DNSRecord
├── Access* (Application, Group, ServiceToken, IdentityProvider, Tunnel)
├── Device* (PostureRule, SettingsPolicy)
├── Gateway* (Rule, List, Configuration)
├── TunnelIngressClassConfig / TunnelGatewayClassConfig
├── OriginCACertificate (新增)
└── ZoneRuleset (新增)

storage.cloudflare-operator.io/v1alpha1 (新增 API Group)
├── R2Bucket
├── R2BucketLifecycle (或内嵌在 R2Bucket)
├── R2BucketCORS (或内嵌在 R2Bucket)
└── R2BucketNotification (或内嵌在 R2Bucket)

registrar.cloudflare-operator.io/v1alpha1 (新增 API Group, Enterprise Only)
├── DomainRegistration
└── DomainTransfer
```

---

## 实现顺序

### Phase 1: v0.20.x (当前)
1. [x] 创建路线图文档
2. [ ] CloudflareDomain SSL 配置
3. [ ] CloudflareDomain 缓存配置
4. [ ] CloudflareDomain 安全配置
5. [ ] CloudflareDomain 性能配置
6. [ ] OriginCACertificate CRD
7. [ ] OriginCACertificate Secret 同步

### Phase 2: v0.21.x
1. [ ] R2Bucket 基础 CRD
2. [ ] R2Bucket 自定义域名
3. [ ] R2Bucket 生命周期规则
4. [ ] R2Bucket CORS
5. [ ] R2Bucket 事件通知

### Phase 3: v0.22.x
1. [ ] ZoneRuleset 基础 CRD
2. [ ] Transform Rules 支持
3. [ ] Redirect Rules 支持
4. [ ] Cache Rules 支持
5. [ ] Origin Rules 支持

### Phase 4: v0.23.x (Enterprise)
1. [ ] DomainRegistration CRD
2. [ ] 域名转入支持
3. [ ] 联系人管理

---

## 参考资源

- [Cloudflare API](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
- [Zone Settings](https://developers.cloudflare.com/api/resources/zones/subresources/settings/)
- [Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)
- [R2 API](https://developers.cloudflare.com/api/resources/r2/)
- [Rulesets Engine](https://developers.cloudflare.com/ruleset-engine/)
- [Registrar](https://developers.cloudflare.com/registrar/)
