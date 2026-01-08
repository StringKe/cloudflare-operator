# Cloudflare Operator E2E 测试计划

## 概述

本文档描述了 cloudflare-operator 的完整端到端测试计划，用于验证所有 CRD 控制器的功能。

## 目标集群

| 项目 | 值 |
|------|-----|
| 集群名称 | your-cluster-name |
| 类型 | Kubernetes |
| Operator 版本 | latest |

## Cloudflare 凭证

| 项目 | 值 |
|------|-----|
| 认证方式 | Global API Key + Email |
| API Key | `${CLOUDFLARE_API_KEY}` |
| Email | `${CLOUDFLARE_EMAIL}` |
| Account ID | `${CLOUDFLARE_ACCOUNT_ID}` |

---

## 控制器实现状态

### 完整实现 (可测试) ✅

| # | CRD | 作用域 | 实现状态 | 依赖 |
|---|-----|--------|----------|------|
| 1 | CloudflareCredentials | Cluster | ✅ 完整 | 无 |
| 2 | VirtualNetwork | Cluster | ✅ 完整 | CloudflareCredentials |
| 3 | ClusterTunnel | Cluster | ✅ 完整 | CloudflareCredentials |
| 4 | Tunnel | Namespaced | ✅ 完整 | CloudflareCredentials |
| 5 | NetworkRoute | Cluster | ✅ 完整 | Tunnel + VirtualNetwork |
| 6 | TunnelBinding | Namespaced | ✅ 完整 | Tunnel + Service |
| 7 | PrivateService | Namespaced | ✅ 完整 | Tunnel + Service + VirtualNetwork |
| 8 | DNSRecord | Namespaced | ✅ 完整 | CloudflareCredentials |
| 9 | AccessApplication | Namespaced | ✅ 完整 | AccessGroup / AccessIdentityProvider |
| 10 | AccessGroup | Cluster | ✅ 完整 | CloudflareCredentials |
| 11 | AccessIdentityProvider | Cluster | ✅ 完整 | CloudflareCredentials |
| 12 | AccessServiceToken | Namespaced | ✅ 完整 | CloudflareCredentials |

### 框架实现 (需验证) ⚠️

| # | CRD | 作用域 | 实现状态 | 说明 |
|---|-----|--------|----------|------|
| 13 | DevicePostureRule | Cluster | ⚠️ 框架完成 | 设备态势规则 |
| 14 | DeviceSettingsPolicy | Cluster | ⚠️ 框架完成 | 设备设置策略 |
| 15 | GatewayList | Cluster | ⚠️ 框架完成 | 网关列表 |
| 16 | GatewayRule | Cluster | ⚠️ 框架完成 | 网关规则 |
| 17 | GatewayConfiguration | Cluster | ⚠️ 框架完成 | 网关配置 |
| 18 | WARPConnector | Cluster | ⚠️ 框架完成 | WARP 连接器 |

---

## 测试阶段

### 阶段 0: 环境准备

#### 0.1 清理现有资源

```bash
# 切换到目标集群
kubectl config use-context arn:aws:eks:ap-south-1:xxx:cluster/mip-1xe-test-aps1

# 删除现有 CR (按逆依赖顺序)
kubectl delete accessapplications.networking.cloudflare-operator.io --all -A
kubectl delete accessservicetokens.networking.cloudflare-operator.io --all -A
kubectl delete accessgroups.networking.cloudflare-operator.io --all
kubectl delete accessidentityproviders.networking.cloudflare-operator.io --all
kubectl delete privateservices.networking.cloudflare-operator.io --all -A
kubectl delete tunnelbindings.networking.cloudflare-operator.io --all -A
kubectl delete networkroutes.networking.cloudflare-operator.io --all
kubectl delete dnsrecords.networking.cloudflare-operator.io --all -A
kubectl delete tunnels.networking.cloudflare-operator.io --all -A
kubectl delete clustertunnels.networking.cloudflare-operator.io --all
kubectl delete virtualnetworks.networking.cloudflare-operator.io --all
kubectl delete cloudflarecredentials.networking.cloudflare-operator.io --all

# 删除 Operator Deployment
kubectl delete deployment -n cloudflare-operator-system cloudflare-operator-controller-manager

# 删除旧 CRD
kubectl delete crd -l app.kubernetes.io/name=cloudflare-operator
```

#### 0.2 安装 Operator v0.17.9

```bash
# 安装 CRD
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/download/v0.17.9/cloudflare-operator.crds.yaml

# 安装 Operator
kubectl apply -f https://github.com/StringKe/cloudflare-operator/releases/download/v0.17.9/cloudflare-operator.yaml

# 等待 Operator 就绪
kubectl wait --for=condition=Available deployment/cloudflare-operator-controller-manager -n cloudflare-operator-system --timeout=120s

# 检查 Operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager -f
```

---

### 阶段 1: 基础设施测试

#### 1.1 CloudflareCredentials (凭证验证)

**文件**: `test/e2e/manifests/01-credentials.yaml`

```yaml
# Secret 包含 API 凭证
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-credentials
  namespace: cloudflare-operator-system
type: Opaque
stringData:
  CLOUDFLARE_API_KEY: "${CLOUDFLARE_API_KEY}"
  CLOUDFLARE_EMAIL: "${CLOUDFLARE_EMAIL}"
---
# CloudflareCredentials 验证凭证
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: CloudflareCredentials
metadata:
  name: default
spec:
  authType: globalAPIKey
  accountId: "${CLOUDFLARE_ACCOUNT_ID}"
  isDefault: true
  secretRef:
    name: cloudflare-api-credentials
    namespace: cloudflare-operator-system
    apiKeyKey: CLOUDFLARE_API_KEY
    emailKey: CLOUDFLARE_EMAIL
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/01-credentials.yaml
kubectl wait --for=jsonpath='{.status.state}'=Ready cloudflarecredentials/default --timeout=60s
kubectl get cloudflarecredentials default -o yaml
```

**预期状态**:
- `.status.state` = `Ready`
- `.status.validated` = `true`
- `.status.accountName` 有值
- `.status.conditions[0].type` = `Ready`
- `.status.conditions[0].status` = `True`

---

#### 1.2 VirtualNetwork (虚拟网络)

**文件**: `test/e2e/manifests/02-virtualnetwork.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: VirtualNetwork
metadata:
  name: test-vnet
spec:
  name: e2e-test-network
  comment: "E2E test virtual network"
  isDefaultNetwork: false
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/02-virtualnetwork.yaml
kubectl wait --for=jsonpath='{.status.state}'=active virtualnetwork/test-vnet --timeout=120s
kubectl get virtualnetwork test-vnet -o yaml
```

**预期状态**:
- `.status.state` = `active`
- `.status.virtualNetworkId` 有值 (UUID)
- `.status.conditions[0].type` = `Ready`
- `.status.conditions[0].status` = `True`

**Cloudflare Dashboard 验证**:
- Zero Trust > Networks > Virtual Networks 中可见 `e2e-test-network`

---

### 阶段 2: 隧道测试

#### 2.1 ClusterTunnel (集群级隧道)

**文件**: `test/e2e/manifests/03-clustertunnel.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: ClusterTunnel
metadata:
  name: test-cluster-tunnel
spec:
  newTunnel:
    name: e2e-test-cluster-tunnel
  enableWarpRouting: true
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    domain: "your-domain.com"  # 替换为实际域名
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/03-clustertunnel.yaml
kubectl wait --for=jsonpath='{.status.tunnelId}'!="" clustertunnel/test-cluster-tunnel --timeout=180s
kubectl get clustertunnel test-cluster-tunnel -o yaml

# 检查 cloudflared Deployment
kubectl get deployment -l app.kubernetes.io/name=cloudflared
kubectl get pods -l app.kubernetes.io/name=cloudflared
```

**预期状态**:
- `.status.tunnelId` 有值 (UUID)
- `.status.tunnelName` = `e2e-test-cluster-tunnel`
- cloudflared Deployment 运行中

**Cloudflare Dashboard 验证**:
- Zero Trust > Networks > Tunnels 中可见隧道，状态为 `Healthy`

---

#### 2.2 Tunnel (命名空间级隧道)

**文件**: `test/e2e/manifests/04-tunnel.yaml`

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: e2e-test
---
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: Tunnel
metadata:
  name: test-tunnel
  namespace: e2e-test
spec:
  newTunnel:
    name: e2e-test-ns-tunnel
  enableWarpRouting: true
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    domain: "your-domain.com"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/04-tunnel.yaml
kubectl wait --for=jsonpath='{.status.tunnelId}'!="" tunnel/test-tunnel -n e2e-test --timeout=180s
kubectl get tunnel test-tunnel -n e2e-test -o yaml
```

---

### 阶段 3: 网络路由测试

#### 3.1 NetworkRoute (网络路由)

**文件**: `test/e2e/manifests/05-networkroute.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: NetworkRoute
metadata:
  name: test-route
spec:
  network: 10.100.0.0/16
  comment: "E2E test network route"
  tunnelRef:
    kind: ClusterTunnel
    name: test-cluster-tunnel
  virtualNetworkRef:
    name: test-vnet
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/05-networkroute.yaml
kubectl wait --for=jsonpath='{.status.state}'=active networkroute/test-route --timeout=120s
kubectl get networkroute test-route -o yaml
```

**预期状态**:
- `.status.state` = `active`
- `.status.network` = `10.100.0.0/16`
- `.status.tunnelID` 有值
- `.status.virtualNetworkID` 有值

---

### 阶段 4: 服务暴露测试

#### 4.1 创建测试应用

**文件**: `test/e2e/manifests/06-test-app.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-test
  namespace: e2e-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx-test
  template:
    metadata:
      labels:
        app: nginx-test
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: nginx-test
  namespace: e2e-test
spec:
  selector:
    app: nginx-test
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
```

#### 4.2 TunnelBinding (服务绑定)

**文件**: `test/e2e/manifests/07-tunnelbinding.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelBinding
metadata:
  name: nginx-binding
  namespace: e2e-test
spec:
  tunnelRef:
    kind: Tunnel
    name: test-tunnel
  subjects:
  - spec:
      target: http://nginx-test.e2e-test.svc.cluster.local:80
      fqdn: nginx-test.your-domain.com
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/07-tunnelbinding.yaml
kubectl get tunnelbinding nginx-binding -n e2e-test -o yaml

# 验证可通过公网访问
curl -s https://nginx-test.your-domain.com
```

---

#### 4.3 PrivateService (私有服务)

**文件**: `test/e2e/manifests/08-privateservice.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: PrivateService
metadata:
  name: nginx-private
  namespace: e2e-test
spec:
  serviceRef:
    name: nginx-test
  tunnelRef:
    kind: ClusterTunnel
    name: test-cluster-tunnel
  virtualNetworkRef:
    name: test-vnet
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/08-privateservice.yaml
kubectl wait --for=jsonpath='{.status.state}'=active privateservice/nginx-private -n e2e-test --timeout=120s
kubectl get privateservice nginx-private -n e2e-test -o yaml
```

**预期状态**:
- `.status.state` = `active`
- `.status.network` = Service ClusterIP + `/32`
- `.status.serviceIP` = Service ClusterIP

---

### 阶段 5: DNS 测试

#### 5.1 DNSRecord

**文件**: `test/e2e/manifests/09-dnsrecord.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: DNSRecord
metadata:
  name: test-dns
  namespace: e2e-test
spec:
  name: e2e-test-record.your-domain.com
  type: A
  content: 1.2.3.4
  ttl: 300
  proxied: false
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/09-dnsrecord.yaml
kubectl wait --for=jsonpath='{.status.state}'=Ready dnsrecord/test-dns -n e2e-test --timeout=60s
kubectl get dnsrecord test-dns -n e2e-test -o yaml

# DNS 查询验证
dig e2e-test-record.your-domain.com
```

---

### 阶段 6: Zero Trust Access 测试

#### 6.1 AccessGroup

**文件**: `test/e2e/manifests/10-accessgroup.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessGroup
metadata:
  name: test-group
spec:
  name: E2E Test Group
  include:
  - emailDomain:
      domain: your-domain.com
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/10-accessgroup.yaml
kubectl wait --for=jsonpath='{.status.state}'=active accessgroup/test-group --timeout=60s
kubectl get accessgroup test-group -o yaml
```

---

#### 6.2 AccessIdentityProvider (OTP)

**文件**: `test/e2e/manifests/11-identityprovider.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessIdentityProvider
metadata:
  name: test-otp
spec:
  name: E2E Test OTP
  type: onetimepin
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/11-identityprovider.yaml
kubectl wait --for=jsonpath='{.status.state}'=active accessidentityprovider/test-otp --timeout=60s
kubectl get accessidentityprovider test-otp -o yaml
```

---

#### 6.3 AccessApplication

**文件**: `test/e2e/manifests/12-accessapplication.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessApplication
metadata:
  name: test-app
  namespace: e2e-test
spec:
  name: E2E Test Application
  domain: e2e-test-app.your-domain.com
  type: self_hosted
  sessionDuration: "24h"
  allowedIdpRefs:
  - name: test-otp
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/12-accessapplication.yaml
kubectl wait --for=jsonpath='{.status.state}'=active accessapplication/test-app -n e2e-test --timeout=60s
kubectl get accessapplication test-app -n e2e-test -o yaml
```

---

#### 6.4 AccessServiceToken

**文件**: `test/e2e/manifests/13-servicetoken.yaml`

```yaml
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: AccessServiceToken
metadata:
  name: test-token
  namespace: e2e-test
spec:
  name: E2E Test Token
  duration: "8760h"  # 1 year
  secretRef:
    name: test-service-token-secret
  cloudflare:
    accountId: "${CLOUDFLARE_ACCOUNT_ID}"
    credentialsRef:
      name: default
```

**验证**:
```bash
kubectl apply -f test/e2e/manifests/13-servicetoken.yaml
kubectl wait --for=jsonpath='{.status.state}'=Ready accessservicetoken/test-token -n e2e-test --timeout=60s
kubectl get accessservicetoken test-token -n e2e-test -o yaml

# 检查 Secret 是否创建
kubectl get secret test-service-token-secret -n e2e-test -o yaml
```

---

### 阶段 7: 删除测试

测试资源删除和 Finalizer 正常移除：

```bash
# 按逆依赖顺序删除
kubectl delete accessservicetoken test-token -n e2e-test
kubectl delete accessapplication test-app -n e2e-test
kubectl delete accessidentityprovider test-otp
kubectl delete accessgroup test-group
kubectl delete dnsrecord test-dns -n e2e-test
kubectl delete privateservice nginx-private -n e2e-test
kubectl delete tunnelbinding nginx-binding -n e2e-test
kubectl delete networkroute test-route
kubectl delete tunnel test-tunnel -n e2e-test
kubectl delete clustertunnel test-cluster-tunnel
kubectl delete virtualnetwork test-vnet
kubectl delete cloudflarecredentials default

# 删除测试命名空间
kubectl delete namespace e2e-test

# 验证 Cloudflare 侧资源已清理
# - Tunnels 已删除
# - Virtual Networks 已删除
# - DNS Records 已删除
# - Access Applications 已删除
# - Access Groups 已删除
# - Service Tokens 已删除
```

---

## 验证检查清单

### 每个 CRD 的通用检查项

- [ ] 资源创建成功
- [ ] `.status.state` = 预期状态 (Ready/active)
- [ ] `.status.conditions[0].type` = `Ready`
- [ ] `.status.conditions[0].status` = `True`
- [ ] `.status.observedGeneration` = `.metadata.generation`
- [ ] Cloudflare Dashboard 显示对应资源
- [ ] 资源删除后 Finalizer 正常移除
- [ ] Cloudflare 侧资源同步删除

### 特殊检查项

| CRD | 额外检查 |
|-----|----------|
| CloudflareCredentials | `.status.validated` = true |
| Tunnel/ClusterTunnel | cloudflared Deployment 运行中 |
| NetworkRoute | WARP 客户端可访问目标网络 |
| TunnelBinding | 公网可访问绑定的服务 |
| PrivateService | WARP 客户端可访问服务 ClusterIP |
| DNSRecord | DNS 解析正确 |
| AccessServiceToken | Secret 包含 Client ID/Secret |

---

## 故障排查

### 查看 Operator 日志

```bash
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager -f
```

### 查看 CR Events

```bash
kubectl describe <resource-type> <name> -n <namespace>
```

### 常见问题

1. **凭证验证失败**
   - 检查 Secret 中的 API Key 和 Email 是否正确
   - 确认 Account ID 与账户匹配

2. **隧道创建失败**
   - 检查 domain 是否在账户下
   - 确认 API Token 有 Tunnel 权限

3. **NetworkRoute 创建失败**
   - 确保引用的 Tunnel 已有 `tunnelId`
   - 确保 `enableWarpRouting: true`

4. **资源删除卡住**
   - 检查 Finalizer 是否存在
   - 查看 Events 中的错误信息
   - 确认 Cloudflare API 可达

---

## 测试结果记录

| # | CRD | 创建 | 状态验证 | CF 验证 | 删除 | 备注 |
|---|-----|------|----------|---------|------|------|
| 1 | CloudflareCredentials | | | N/A | | |
| 2 | VirtualNetwork | | | | | |
| 3 | ClusterTunnel | | | | | |
| 4 | Tunnel | | | | | |
| 5 | NetworkRoute | | | | | |
| 6 | TunnelBinding | | | | | |
| 7 | PrivateService | | | | | |
| 8 | DNSRecord | | | | | |
| 9 | AccessGroup | | | | | |
| 10 | AccessIdentityProvider | | | | | |
| 11 | AccessApplication | | | | | |
| 12 | AccessServiceToken | | | | | |

**测试日期**: _______________
**测试人员**: _______________
**测试结论**: _______________
