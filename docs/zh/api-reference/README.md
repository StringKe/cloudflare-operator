# API 参考

本节包含所有自定义资源定义（CRD）的详细文档。

## CRD 分类

### 隧道管理
- [Tunnel](tunnel.md) - 命名空间级 Cloudflare Tunnel
- [ClusterTunnel](clustertunnel.md) - 集群级 Cloudflare Tunnel
- [TunnelBinding](tunnelbinding.md) - 将服务绑定到隧道

### 私有网络
- [VirtualNetwork](virtualnetwork.md) - 流量隔离网络
- [NetworkRoute](networkroute.md) - 通过隧道路由 CIDR
- [PrivateService](privateservice.md) - 私有 IP 服务暴露
- [WARPConnector](warpconnector.md) - 站点间 WARP 连接器

### 访问控制
- [AccessApplication](accessapplication.md) - Zero Trust 应用
- [AccessGroup](accessgroup.md) - 可复用的访问策略组
- [AccessIdentityProvider](accessidentityprovider.md) - 身份提供商配置
- [AccessServiceToken](accessservicetoken.md) - M2M 认证令牌
- [AccessTunnel](accesstunnel.md) - Access 保护的隧道端点

### 网关与安全
- [GatewayRule](gatewayrule.md) - DNS/HTTP/L4 策略规则
- [GatewayList](gatewaylist.md) - 网关规则使用的列表
- [GatewayConfiguration](gatewayconfiguration.md) - 全局网关设置

### 设备管理
- [DeviceSettingsPolicy](devicesettingspolicy.md) - WARP 客户端配置
- [DevicePostureRule](deviceposturerule.md) - 设备健康检查规则

### DNS 与连接
- [DNSRecord](dnsrecord.md) - DNS 记录管理

### Pages 与 Workers
- [PagesProject](pagesproject.md) - Cloudflare Pages 项目管理
- [PagesDeployment](pagesdeployment.md) - 部署版本到 Pages
- [PagesDomain](pagesdomain.md) - Pages 自定义域名

### Kubernetes 集成
- [TunnelIngressClassConfig](tunnelingressclassconfig.md) - Ingress 集成
- [TunnelGatewayClassConfig](tunnelgatewayclassconfig.md) - Gateway API 集成

## 通用类型

### CloudflareSpec

所有与 Cloudflare API 交互的 CRD 都包含 `cloudflare` 规格：

```yaml
spec:
  cloudflare:
    accountId: "your-account-id"
    domain: example.com
    secret: cloudflare-credentials
```

### 状态条件

所有 CRD 通过标准 Kubernetes 条件报告状态：

| 条件 | 说明 |
|------|------|
| `Ready` | 资源已完全调和并可操作 |
| `Progressing` | 资源正在创建或更新中 |
| `Degraded` | 资源有错误但可能部分可用 |

## API 版本

当前 API 版本：`networking.cloudflare-operator.io/v1alpha2`

旧版本 `v1alpha1` 已弃用但仍支持向后兼容。
