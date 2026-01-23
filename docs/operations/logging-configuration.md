# 日志配置指南

## 问题说明

### 为什么部署后所有日志都显示为 ERROR？

默认情况下，cloudflare-operator 使用 **console encoder**（文本格式）输出日志：

```
2026-01-22T22:14:32+05:30    DEBUG    Processing TunnelConfiguration SyncState    {"controller": "tunnel-config-sync", ...}
```

这种格式对于人类阅读友好，但**不适合日志聚合系统**（如 Datadog、ELK、Loki）：

- Datadog 等系统期望接收**标准 JSON 格式**的日志
- 无法从文本格式中正确解析日志级别（DEBUG/INFO/ERROR）
- 当无法识别级别时，默认将所有日志标记为 `ERROR`

### 解决方案

启用 **JSON encoder** 输出结构化日志：

```json
{
  "level": "debug",
  "ts": "2026-01-22T22:14:32.123+05:30",
  "msg": "Processing TunnelConfiguration SyncState",
  "controller": "tunnel-config-sync",
  "controllerGroup": "networking.cloudflare-operator.io",
  "controllerKind": "CloudflareSyncState",
  "CloudflareSyncState": {"name":"tunnel-configuration-7f51517d-e1f1-43f5-aae8-7a2f72fbc07d"},
  "name": "tunnel-configuration-7f51517d-e1f1-43f5-aae8-7a2f72fbc07d",
  "cloudflareId": "7f51517d-e1f1-43f5-aae8-7a2f72fbc07d"
}
```

## 日志配置选项

cloudflare-operator 基于 `controller-runtime/log/zap`，支持以下参数：

| 参数 | 默认值 | 说明 | 推荐值 |
|------|--------|------|--------|
| `--zap-encoder` | `console` | 日志编码器：`console` 或 `json` | 生产：`json`<br>开发：`console` |
| `--zap-devel` | `true` | 开发模式（启用 DPanic 级别） | 生产：`false`<br>开发：`true` |
| `--zap-log-level` | `info` | 最低日志级别：`debug`/`info`/`error` | 生产：`info`<br>调试：`debug` |
| `--zap-stacktrace-level` | `error` | 打印 stacktrace 的最低级别 | `error` |
| `--zap-time-encoding` | - | 时间格式：`iso8601`/`epoch`/`millis` | `iso8601` |

## 部署配置

### 方式 1：修改 Deployment YAML（已内置）

项目已在 `config/manager/manager.yaml` 中配置生产级日志参数：

```yaml
args:
  - --leader-elect
  - --health-probe-bind-address=:8081
  # JSON 格式日志，用于 Datadog 等日志聚合系统
  - --zap-encoder=json
  # 生产环境建议关闭 Development 模式
  - --zap-devel=false
  # 设置日志级别（debug/info/error）
  - --zap-log-level=info
  # 仅在 error 级别打印 stacktrace
  - --zap-stacktrace-level=error
```

部署后自动生效。

### 方式 2：通过环境变量覆盖（运行时调整）

如需临时调整日志级别，可使用 Kustomize overlay：

```yaml
# config/overlays/debug/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../default
patches:
  - target:
      kind: Deployment
      name: controller-manager
    patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/args/3
        value: --zap-log-level=debug
```

### 方式 3：Helm Values（如果使用 Helm）

```yaml
# values.yaml
manager:
  args:
    - --zap-encoder=json
    - --zap-devel=false
    - --zap-log-level=info
    - --zap-stacktrace-level=error
```

## 验证配置

### 1. 检查 Pod 参数

```bash
kubectl --context <your-context> get deployment -n cloudflare-operator-system \
  cloudflare-operator-controller-manager -o yaml | grep -A 10 args:
```

期望输出：

```yaml
args:
- --leader-elect
- --health-probe-bind-address=:8081
- --zap-encoder=json
- --zap-devel=false
- --zap-log-level=info
- --zap-stacktrace-level=error
```

### 2. 检查日志格式

```bash
kubectl --context <your-context> logs -n cloudflare-operator-system \
  deployment/cloudflare-operator-controller-manager -c manager --tail 10
```

**JSON 格式示例**（✅ 正确）：

```json
{"level":"info","ts":"2026-01-22T17:00:00.000Z","msg":"starting manager"}
{"level":"debug","ts":"2026-01-22T17:00:01.123Z","msg":"Processing SyncState","controller":"tunnel-config-sync"}
```

**文本格式示例**（❌ 会导致 Datadog 误判）：

```
2026-01-22T17:00:00+00:00    INFO    starting manager
2026-01-22T17:00:01+00:00    DEBUG   Processing SyncState    {"controller": "tunnel-config-sync"}
```

### 3. 验证 Datadog 日志级别

在 Datadog 中检查日志：

- ✅ 正确：日志按 `info`/`debug`/`error` 分类
- ❌ 错误：所有日志都显示为 `error`

## 日志级别选择指南

| 环境 | 推荐配置 | 理由 |
|------|----------|------|
| **生产环境** | `--zap-log-level=info` | 平衡可观测性与性能 |
| **测试环境** | `--zap-log-level=info` | 与生产保持一致 |
| **开发环境** | `--zap-log-level=debug` | 详细调试信息 |
| **故障排查** | `--zap-log-level=debug` | 临时启用，排查后恢复 |

## 高级配置：结构化日志字段

JSON 日志自动包含以下结构化字段：

```json
{
  "level": "info",               // 日志级别
  "ts": "2026-01-22T17:00:00Z",  // ISO 8601 时间戳
  "msg": "reconciling resource", // 日志消息
  "controller": "tunnel",        // 控制器名称
  "controllerKind": "Tunnel",    // K8s 资源类型
  "namespace": "default",        // 命名空间
  "name": "my-tunnel",           // 资源名称
  "reconcileID": "abc123"        // 协调标识符
}
```

这些字段可在 Datadog 中用于：

- **过滤**：`controller:tunnel AND level:error`
- **聚合**：按 `namespace` 或 `controllerKind` 分组
- **告警**：基于 `level:error` 创建监控

## 性能影响

| 配置 | CPU 影响 | 内存影响 | 日志量 |
|------|---------|---------|--------|
| `debug` + `json` | +5-10% | +10-15 MB | 5-10x |
| `info` + `json` | +1-2% | +5 MB | 1x (基线) |
| `info` + `console` | 基线 | 基线 | 1x |

**建议**：
- 生产环境使用 `info` + `json`
- 临时调试使用 `debug`，完成后立即恢复 `info`

## 故障排查

### 问题：修改后仍显示文本格式

**检查步骤**：

1. 确认 ConfigMap/Secret 没有覆盖参数
2. 重启 Pod：`kubectl rollout restart -n cloudflare-operator-system deployment/cloudflare-operator-controller-manager`
3. 检查 Pod 启动参数：`kubectl get pod -n cloudflare-operator-system <pod-name> -o yaml | grep args -A 10`

### 问题：JSON 日志但 Datadog 仍显示 ERROR

**可能原因**：

1. Datadog Agent 配置未识别 `level` 字段
2. 检查 Datadog Pipeline：确保 `level` 映射到 `status`

**解决方案**：

```yaml
# datadog-agent ConfigMap
logs_config:
  processing_rules:
    - type: remap_severity
      name: remap_level_to_status
      pattern: '\"level\":\"([^\"]+)\"'
```

## 参考资料

- [controller-runtime Logging](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/log/zap)
- [Zap Logger Documentation](https://pkg.go.dev/go.uber.org/zap)
- [Datadog Kubernetes Log Collection](https://docs.datadoghq.com/agent/kubernetes/log/)
