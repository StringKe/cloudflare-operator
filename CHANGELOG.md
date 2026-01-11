# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.21.0] - 2026-01-11

### Added
- **Type Safety Improvements**: Replaced all `interface{}`/`any` types with precise typed structs
- 30+ typed structs for Access rules (email, SAML, OIDC, GitHub, Azure, Okta, device posture)
- Complete typed structs for Gateway rule settings (L4 override, BISO controls, DNS resolvers)
- DNSRecordDataParams covering all DNS record types (SRV, CAA, CERT, SSHFP, TLSA, LOC, URI)
- Generic `r2RulesRequest[T]` for type-safe R2 API calls
- 200+ new unit tests for type conversion functions
- New `cloudflare-api-schema-guide.md` for SDK type mapping reference

### Changed
- All controllers now use type-safe parameter building
- Removed unsafe type assertions and interface{} usage
- Better error handling with proper type checking
- Improved API type definitions with kubebuilder validation markers

### Fixed
- Lint issues in test files and API types

## [0.20.0] - 2026-01-11

### Added
- **R2 Storage CRDs**:
  - `R2Bucket` - R2 storage bucket management with lifecycle rules
  - `R2BucketDomain` - Custom domain configuration for R2 buckets
  - `R2BucketNotification` - Event notifications for R2 buckets
- **Rules Engine CRDs**:
  - `ZoneRuleset` - Zone ruleset management (WAF, rate limiting, etc.)
  - `TransformRule` - URL rewrite and header modification rules
  - `RedirectRule` - URL redirect rules
- **SSL/TLS CRD**:
  - `OriginCACertificate` - Cloudflare Origin CA certificate with automatic K8s Secret creation
- **Registrar CRD**:
  - `DomainRegistration` - Domain registration settings (Enterprise)
- OpenSSF Scorecard security compliance improvements
- GitHub Actions security with pinned SHA hashes
- Minimal token permissions per workflow job

### Fixed
- Case-sensitive filename collision for CloudflareDomain CRD

## [0.19.5] - 2026-01-10

### Fixed
- CI/CD workflow issues

## [0.19.4] - 2026-01-10

### Fixed
- CI/CD workflow issues

## [0.19.3] - 2026-01-10

### Fixed
- CI/CD workflow issues

## [0.19.2] - 2026-01-10

### Fixed
- DNS record creation with trailing dots in domain validation
- DNSRecord controller to respect `spec.cloudflare.domain` field for zone resolution
- Restored correct tool versions in Makefile

### Added
- Domain validation to prevent wrong zone record creation

## [0.19.1] - 2026-01-10

### Fixed
- Race condition in CloudflareDomain state changes

### Added
- CRD creation checklist to documentation

## [0.19.0] - 2026-01-10

### Added
- **CloudflareDomain CRD** for multi-zone DNS support
  - Zone settings (SSL/TLS, Cache, Security, WAF)
  - Zone-level configuration management
- Multi-zone support for DNSRecord resources

## [0.18.5] - 2026-01-10

### Fixed
- Ingress controller to set ValidTunnelId in API client for DNS operations

## [0.18.4] - 2026-01-10

### Fixed
- Ingress controller JSON tags for Configuration structs (sigs.k8s.io/yaml compatibility)

## [0.18.3] - 2026-01-10

### Fixed
- Included TunnelIngressClassConfig and TunnelGatewayClassConfig in CRD release

## [0.18.2] - 2026-01-10

### Changed
- Updated documentation for v0.18.x release

## [0.18.1] - 2026-01-10

### Fixed
- Various stability improvements

## [0.18.0] - 2026-01-10

### Added
- **Kubernetes Integration CRDs**:
  - `TunnelIngressClassConfig` - Configuration for Kubernetes Ingress integration
  - `TunnelGatewayClassConfig` - Configuration for Gateway API integration
- Native Kubernetes Ingress controller support
- Gateway API support (Gateway, HTTPRoute, TCPRoute, UDPRoute)

### Changed
- Major refactoring of tunnel configuration management
- Improved route building and path handling

## [0.17.x] and earlier

See [GitHub Releases](https://github.com/StringKe/cloudflare-operator/releases) for historical changes.

---

## Version Summary (v0.18.0 → v0.21.0)

### New CRDs Added (9 total)

| CRD | Version | Purpose |
|-----|---------|---------|
| CloudflareDomain | v0.19.0 | Multi-zone DNS support, Zone-level configuration |
| R2Bucket | v0.20.0 | R2 storage bucket management with lifecycle rules |
| R2BucketDomain | v0.20.0 | Custom domains for R2 buckets |
| R2BucketNotification | v0.20.0 | Event notifications for R2 buckets |
| ZoneRuleset | v0.20.0 | WAF, rate limiting, and zone rulesets |
| TransformRule | v0.20.0 | URL/Header transformation rules |
| RedirectRule | v0.20.0 | URL redirect rules |
| OriginCACertificate | v0.20.0 | Origin CA certificate management |
| DomainRegistration | v0.20.0 | Domain registration (Enterprise) |

### Statistics

- **Total Commits**: 25+
- **Lines Added**: ~10,000+
- **Total CRDs**: 30
- **New Unit Tests**: 200+

[Unreleased]: https://github.com/StringKe/cloudflare-operator/compare/v0.21.0...HEAD
[0.21.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.5...v0.20.0
[0.19.5]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.4...v0.19.5
[0.19.4]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.3...v0.19.4
[0.19.3]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.2...v0.19.3
[0.19.2]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.1...v0.19.2
[0.19.1]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.5...v0.19.0
[0.18.5]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.4...v0.18.5
[0.18.4]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.3...v0.18.4
[0.18.3]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.2...v0.18.3
[0.18.2]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.1...v0.18.2
[0.18.1]: https://github.com/StringKe/cloudflare-operator/compare/v0.18.0...v0.18.1
[0.18.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.17.13...v0.18.0
