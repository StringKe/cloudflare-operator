# 故障排除

本指南帮助诊断和解决 Cloudflare Operator 的常见问题。

## 诊断命令

### 检查 Operator 状态

```bash
# Operator pod 状态
kubectl get pods -n cloudflare-operator-system

# Operator 日志
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager

# 启用调试日志
kubectl patch deployment cloudflare-operator-controller-manager \
  -n cloudflare-operator-system \
  --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--zap-log-level=debug"}]'
```

### 检查资源状态

```bash
# 列出所有 operator 资源
kubectl get tunnels,clustertunnels,tunnelbindings,virtualnetworks,networkroutes -A

# 带条件的详细状态
kubectl get tunnel <name> -o jsonpath='{.status.conditions}' | jq

# 资源事件
kubectl describe tunnel <name>
```

## 常见问题

### 隧道无法连接

**症状：**
- 隧道状态显示错误
- cloudflared pods 未运行或 CrashLooping

**诊断步骤：**

```bash
# 检查隧道状态
kubectl get tunnel <name> -o wide

# 检查 cloudflared 部署
kubectl get deployment -l app.kubernetes.io/name=cloudflared

# 检查 cloudflared 日志
kubectl logs -l app.kubernetes.io/name=cloudflared
```

**常见原因：**

1. **API Token 无效**
   ```bash
   # 验证 token 是否有效
   curl -X GET "https://api.cloudflare.com/client/v4/user/tokens/verify" \
     -H "Authorization: Bearer $(kubectl get secret cloudflare-credentials -o jsonpath='{.data.CLOUDFLARE_API_TOKEN}' | base64 -d)"
   ```

2. **Account ID 错误**
   - 在 Cloudflare 控制台验证 account ID
   - 检查隧道规格中的拼写

3. **网络连接问题**
   - 确保 pods 能访问 `api.cloudflare.com` 和 `*.cloudflareaccess.com`
   - 检查网络策略

4. **Secret 未找到**
   ```bash
   kubectl get secret <secret-name> -n <namespace>
   ```

### DNS 记录未创建

**症状：**
- TunnelBinding 显示成功但 DNS 无法解析
- Cloudflare 控制台中没有 CNAME 记录

**诊断步骤：**

```bash
# 检查 TunnelBinding 状态
kubectl describe tunnelbinding <name>

# 检查 operator 日志中的 DNS 错误
kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager | grep -i dns
```

**常见原因：**

1. **缺少 DNS:Edit 权限**
   - 检查 token 是否具有该区域的 `Zone:DNS:Edit` 权限

2. **域名错误**
   - 验证 `cloudflare.domain` 是否与你的 Cloudflare 区域匹配

3. **区域未找到**
   - 域名必须在你的 Cloudflare 账户中处于活动状态

### 网络路由不工作

**症状：**
- WARP 客户端无法访问路由的 IP
- 流量未通过隧道

**诊断步骤：**

```bash
# 检查 NetworkRoute 状态
kubectl get networkroute <name> -o wide

# 验证隧道是否启用了 WARP 路由
kubectl get clustertunnel <name> -o jsonpath='{.spec.enableWarpRouting}'
```

**常见原因：**

1. **WARP 路由未启用**
   ```yaml
   spec:
     enableWarpRouting: true  # 必须为 true
   ```

2. **路由冲突**
   - 在 Cloudflare 控制台检查是否有重叠路由
   - 验证 CIDR 不与现有路由冲突

3. **虚拟网络不匹配**
   - WARP 客户端必须连接到正确的虚拟网络

### Access 应用未保护

**症状：**
- 应用无需认证即可访问
- 登录页面未显示

**诊断步骤：**

```bash
# 检查 AccessApplication 状态
kubectl get accessapplication <name> -o wide
kubectl describe accessapplication <name>
```

**常见原因：**

1. **DNS 未指向隧道**
   - 应用域名必须通过 Cloudflare Tunnel 提供服务

2. **策略配置错误**
   - 检查 AccessGroup 规则是否正确
   - 验证策略决策是 `allow` 而不是 `bypass`

3. **缺少 IdP 配置**
   - 必须配置并引用 AccessIdentityProvider

### 资源卡在删除中

**症状：**
- 资源设置了 `DeletionTimestamp`
- Finalizers 阻止删除

**诊断步骤：**

```bash
# 检查 finalizers
kubectl get <resource> <name> -o jsonpath='{.metadata.finalizers}'

# 检查删除错误
kubectl describe <resource> <name>
```

**解决方案：**

1. **检查 Cloudflare API 错误**
   - 资源可能已在 Cloudflare 中删除
   - Operator 会重试删除

2. **手动删除 Finalizer**（谨慎使用）
   ```bash
   kubectl patch <resource> <name> -p '{"metadata":{"finalizers":null}}' --type=merge
   ```

## 错误消息

### "API Token validation failed"

- Token 无效或已过期
- 在 Cloudflare 控制台重新创建 token

### "Zone not found"

- 域名不在你的 Cloudflare 账户中
- 域名未激活（等待名称服务器更改）

### "Conflict: resource already exists"

- Cloudflare 中已存在同名资源
- 使用 `existingTunnel` 采用现有资源

### "Permission denied"

- Token 缺少所需权限
- 检查[权限矩阵](configuration.md#权限矩阵)

## 获取帮助

如果问题持续存在：

1. **收集日志**
   ```bash
   kubectl logs -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager > operator.log
   ```

2. **检查 GitHub Issues**
   - 搜索[现有问题](https://github.com/StringKe/cloudflare-operator/issues)

3. **开新 Issue**
   - 包含 operator 版本
   - 包含相关 CRD 清单（已脱敏）
   - 包含错误日志
