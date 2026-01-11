# Cloudflare API Schema 查询指南

本文档描述如何查询 Cloudflare OpenAPI Schema 来获取精确的类型定义。

## 1. 下载 OpenAPI Schema

```bash
# 下载完整的 OpenAPI schema (约 34 万行)
curl -sL "https://raw.githubusercontent.com/cloudflare/api-schemas/main/openapi.yaml" -o /tmp/cloudflare-openapi.yaml
```

## 2. 常用查询命令

### 2.1 查找 Schema 定义位置

```bash
# 查找特定 schema 的行号
grep -n "^        access_rule:\|^        access_include:" /tmp/cloudflare-openapi.yaml

# 查找所有 Access 相关的 schema
grep -n "^        access_" /tmp/cloudflare-openapi.yaml | head -50
```

### 2.2 查看 Schema 内容

```bash
# 查看特定行号范围的内容
sed -n '9476,9510p' /tmp/cloudflare-openapi.yaml

# 查看 access_rule 的完整定义 (oneOf 联合类型)
```

### 2.3 查找规则类型定义

```bash
# 查找所有规则类型
grep -n "^        access_.*_rule:" /tmp/cloudflare-openapi.yaml
```

## 3. 核心 Schema 定义

### 3.1 Access Rule (联合类型)

位置: 行 9476

```yaml
access_rule:
  oneOf:
    - $ref: '#/components/schemas/access_access_group_rule'
    - $ref: '#/components/schemas/access_any_valid_service_token_rule'
    - $ref: '#/components/schemas/access_auth_context_rule'
    - $ref: '#/components/schemas/access_authentication_method_rule'
    - $ref: '#/components/schemas/access_azure_group_rule'
    - $ref: '#/components/schemas/access_certificate_rule'
    - $ref: '#/components/schemas/access_common_name_rule'
    - $ref: '#/components/schemas/access_country_rule'
    - $ref: '#/components/schemas/access_device_posture_rule'
    - $ref: '#/components/schemas/access_domain_rule'
    - $ref: '#/components/schemas/access_email_list_rule'
    - $ref: '#/components/schemas/access_email_rule'
    - $ref: '#/components/schemas/access_everyone_rule'
    - $ref: '#/components/schemas/access_external_evaluation_rule'
    - $ref: '#/components/schemas/access_github_organization_rule'
    - $ref: '#/components/schemas/access_gsuite_group_rule'
    - $ref: '#/components/schemas/access_login_method_rule'
    - $ref: '#/components/schemas/access_ip_list_rule'
    - $ref: '#/components/schemas/access_ip_rule'
    - $ref: '#/components/schemas/access_okta_group_rule'
    - $ref: '#/components/schemas/access_saml_group_rule'
    - $ref: '#/components/schemas/access_oidc_claim_rule'
    - $ref: '#/components/schemas/access_service_token_rule'
    - $ref: '#/components/schemas/access_linked_app_token_rule'
  type: object
```

### 3.2 各规则类型的 JSON 结构

| 规则类型 | JSON Key | 内部结构 |
|----------|----------|----------|
| access_group_rule | `group` | `{id: string}` |
| any_valid_service_token_rule | `any_valid_service_token` | `{}` (空对象) |
| auth_context_rule | `auth_context` | `{ac_id, id, identity_provider_id}` |
| authentication_method_rule | `auth_method` | `{auth_method: string}` |
| azure_group_rule | `azureAD` | `{id, identity_provider_id}` |
| certificate_rule | `certificate` | `{}` (空对象) |
| common_name_rule | `common_name` | `{common_name: string}` |
| country_rule | `geo` | `{country_code: string}` |
| device_posture_rule | `device_posture` | `{integration_uid: string}` |
| domain_rule | `email_domain` | `{domain: string}` |
| email_list_rule | `email_list` | `{id: string}` |
| email_rule | `email` | `{email: string}` |
| everyone_rule | `everyone` | `{}` (空对象) |
| external_evaluation_rule | `external_evaluation` | `{evaluate_url, keys_url}` |
| github_organization_rule | `github-organization` | `{name, identity_provider_id, team?}` |
| gsuite_group_rule | `gsuite` | `{email, identity_provider_id}` |
| login_method_rule | `login_method` | `{id: string}` |
| ip_list_rule | `ip_list` | `{id: string}` |
| ip_rule | `ip` | `{ip: string}` |
| okta_group_rule | `okta` | `{name, identity_provider_id}` |
| saml_group_rule | `saml` | `{attribute_name, attribute_value, identity_provider_id}` |
| oidc_claim_rule | `oidc` | `{claim_name, claim_value, identity_provider_id}` |
| service_token_rule | `service_token` | `{token_id: string}` |
| linked_app_token_rule | `linked_app_token` | `{app_uid: string}` |

### 3.3 Device Posture Rule Input

位置: 搜索 `devices_device_posture_rules`

```bash
grep -n "devices_posture_input\|device_posture_input" /tmp/cloudflare-openapi.yaml
```

### 3.4 Gateway Rule Settings

位置: 搜索 `gateway_rule_settings`

```bash
grep -n "gateway.*settings\|gateway_rule" /tmp/cloudflare-openapi.yaml
```

### 3.5 DNS Record Data

位置: 搜索 `dns_record_data`

```bash
grep -n "dns.*data\|dns_record" /tmp/cloudflare-openapi.yaml
```

## 4. 使用 jq/yq 进行复杂查询

如果安装了 yq (YAML processor):

```bash
# 安装 yq
brew install yq

# 提取特定 schema
yq '.components.schemas.access_rule' /tmp/cloudflare-openapi.yaml

# 列出所有 access 相关 schemas
yq '.components.schemas | keys | .[] | select(. | test("^access_"))' /tmp/cloudflare-openapi.yaml
```

## 5. 最佳实践

1. **避免使用 `interface{}` 或 `any`**: 始终使用精确类型
2. **创建联合类型结构体**: 对于 oneOf 类型，创建包含所有可选字段的结构体
3. **JSON 标签**: 确保 JSON 标签与 API 规范一致
4. **参考 CRD 类型**: 查看 `api/v1alpha2/` 中已定义的类型作为参考

## 6. 已定义的精确类型

### AccessGroupRule (api/v1alpha2/accessgroup_types.go)

```go
type AccessGroupRule struct {
    Email              *AccessGroupEmailRule
    EmailDomain        *AccessGroupEmailDomainRule
    EmailList          *AccessGroupEmailListRule
    Everyone           bool
    IPRanges           *AccessGroupIPRangesRule
    IPList             *AccessGroupIPListRule
    Country            *AccessGroupCountryRule
    Group              *AccessGroupGroupRule
    ServiceToken       *AccessGroupServiceTokenRule
    AnyValidServiceToken bool
    Certificate        bool
    CommonName         *AccessGroupCommonNameRule
    DevicePosture      *AccessGroupDevicePostureRule
    GSuite             *AccessGroupGSuiteRule
    GitHub             *AccessGroupGitHubRule
    Azure              *AccessGroupAzureRule
    Okta               *AccessGroupOktaRule
    OIDC               *AccessGroupOIDCRule
    SAML               *AccessGroupSAMLRule
    AuthMethod         *AccessGroupAuthMethodRule
    AuthContext        *AccessGroupAuthContextRule
    LoginMethod        *AccessGroupLoginMethodRule
    ExternalEvaluation *AccessGroupExternalEvaluationRule
}
```

## 7. Gateway Rule Settings (zero-trust-gateway_rule-settings)

位置: 行 76866

主要字段:

| 字段名 | 类型 | 描述 |
|--------|------|------|
| add_headers | `map[string][]string` | 添加自定义请求头 |
| allow_child_bypass | `bool` | 允许子 MSP 账户绕过 |
| audit_ssh | `{command_logging: bool}` | SSH 审计设置 |
| biso_admin_controls | `object` | 浏览器隔离控制 |
| block_page_enabled | `bool` | 启用自定义阻止页面 |
| block_reason | `string` | 阻止原因 |
| bypass_parent_rule | `bool` | 绕过父规则 |
| check_session | `{enforce: bool, duration: string}` | 会话检查 |
| dns_resolvers | `{ipv4: [], ipv6: []}` | 自定义 DNS 解析器 |
| egress | `{ipv4, ipv6, ipv4_fallback}` | 出口设置 |
| l4override | `{ip: string, port: int}` | L4 覆盖 |
| notification_settings | `object` | 通知设置 |
| override_host | `string` | DNS 覆盖主机 |
| override_ips | `[]string` | DNS 覆盖 IP |
| payload_log | `{enabled: bool}` | DLP 负载日志 |
| quarantine | `{file_types: []string}` | 隔离设置 |
| resolve_dns_internally | `{view_id, fallback}` | 内部 DNS 解析 |
| resolve_dns_through_cloudflare | `bool` | 通过 Cloudflare 解析 |
| untrusted_cert | `{action: string}` | 不可信证书处理 |

## 8. Device Posture Rule Input

搜索命令:
```bash
grep -n "device_posture\|posture_rule" /tmp/cloudflare-openapi.yaml
```

已在 CRD 中定义为 `DevicePostureInput` 结构体。

## 9. DNS Record Data

搜索命令:
```bash
grep -n "dns_record" /tmp/cloudflare-openapi.yaml
```

已在 CRD 中定义为 `DNSRecordData` 结构体。

## 10. cloudflare-go SDK 类型映射

| OpenAPI Schema | SDK 类型 (v0.x) | CRD 类型 |
|----------------|-----------------|----------|
| access_rule | `interface{}` (联合类型) | `AccessGroupRule` |
| zero-trust-gateway_rule-settings | `TeamsRuleSettings` | `GatewayRuleSettings` |
| access_groups.include/exclude/require | `[]interface{}` | `[]AccessGroupRule` |

## 11. 参考链接

- [Cloudflare API Schemas (GitHub)](https://github.com/cloudflare/api-schemas)
- [Cloudflare API Documentation](https://developers.cloudflare.com/api/)
- [cloudflare-go SDK](https://github.com/cloudflare/cloudflare-go)
- [DeepWiki - cloudflare-go](https://deepwiki.com/cloudflare/cloudflare-go)
