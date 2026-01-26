# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.38.0] - 2026-01-27

### Breaking Changes
- **AccessApplication**: Remove `allowedIdpRefs` field, use `identityProviderRefs` instead
- **AccessApplication**: Remove `destinations[].vnetId` field, use `destinations[].vnetRef` instead
- **AccessGroup**: Remove `identityProviderId` from all rule types (GSuite, GitHub, Azure, Okta, OIDC, SAML, AuthContext, LoginMethod), use `idpRef` instead
- **AccessServiceToken**: Remove `secretRef.namespace` field, Secret is automatically created in the resource namespace

### Added
- **PagesDomain**: Implement `autoConfigureDNS` feature for automatic CNAME record creation
- **PagesDomain**: Extract and expose `validationMethod` and `validationStatus` in Status

### Fixed
- **PagesDomain**: Fix API type conversion to properly extract validation data from Cloudflare response

## [0.27.6] - 2026-01-20

### Changed
- **docs**: Update API permissions matrix with detailed per-CRD requirements
- **docs**: Remove OpenSSF Scorecard badge from README

## [0.27.5] - 2026-01-19

### Added
- **feat**: CRD completeness improvements with AccessApplication Watch for AccessPolicy
- **feat**: TunnelBinding migration tooling and documentation
- **feat**: TunnelGatewayClassConfig DNS management implementation

### Changed
- **ci**: Use matrix build for multi-arch Docker images (amd64 + arm64)

## [0.27.4] - 2026-01-19

### Changed
- **ci**: Add native multi-arch support using ARM64 runners
- **ci**: Simplify CI/CD pipeline for faster releases

## [0.27.3] - 2026-01-18

### Fixed
- **syncstate**: Sanitize label values to remove invalid characters

### Changed
- **docs**: Update documentation for v0.27.x features

## [0.27.2] - 2026-01-18

### Added
- **pages**: Advanced deployment features (Direct Upload, Smart Rollback, Project Adoption)
  - HTTP/S3/OCI source support for Direct Upload
  - Checksum verification and archive extraction
  - Three rollback strategies: LastSuccessful, ByVersion, ExactDeploymentID
  - Project adoption with MustNotExist/IfExists/MustExist policies

## [0.27.1] - 2026-01-17

### Fixed
- **networkroute**: Fix VirtualNetworkID adoption and deletion logic

## [0.27.0] - 2026-01-17

### Added
- **access**: Inline include/exclude/require rules support for AccessApplication policies
- **access**: Comprehensive tests and documentation for inline policies

### Changed
- **docs**: Update documentation for v0.24.0-v0.26.0 releases

## [0.26.0] - 2026-01-16

### Added
- **Cloudflare Pages CRDs** with full six-layer architecture:
  - `PagesProject` - Pages project management with build config and resource bindings
  - `PagesDomain` - Custom domain configuration for Pages projects
  - `PagesDeployment` - Deployment management (create, retry, rollback)

## [0.25.0] - 2026-01-15

### Added
- **AccessPolicy CRD** for reusable access policies
  - Can be referenced by multiple AccessApplication resources
  - Supports all access rule types (email, SAML, OIDC, GitHub, Azure, etc.)

### Fixed
- **lint**: Resolve deprecatedComment and cognitive-complexity issues

## [0.24.0] - 2026-01-14

### Added
- **sync**: Unified aggregation pattern for L5 sync controllers
- **sync**: Complete L5 deletion handling for state consistency
- **test**: Comprehensive unit and E2E tests for L5 sync controllers

### Fixed
- **e2e**: Resolve mock server integration for E2E tests
- **e2e**: Correct API type field names in E2E tests

## [0.23.2] - 2026-01-13

### Added
- **build**: Modular YAML installer architecture
  - `cloudflare-operator-full.yaml` - Complete installation
  - `cloudflare-operator-full-no-webhook.yaml` - Without webhook
  - `cloudflare-operator-crds.yaml` - CRDs only
  - `cloudflare-operator-namespace.yaml` - Namespace only

## [0.23.1] - 2026-01-13

### Changed
- **build(deps)**: Update Go and GitHub Actions dependencies

### Fixed
- **mockserver**: Correct CreateTunnelRoute API path to match cloudflare-go SDK

## [0.23.0] - 2026-01-12

### Added
- **e2e**: Comprehensive E2E test framework improvements
- **controller**: Unified credentials resolution pattern
- **test**: Comprehensive test infrastructure with mock server

### Fixed
- **e2e**: Correct field names and remove unused imports
- **lint**: Resolve all golangci-lint errors

## [0.22.3] - 2026-01-12

### Added
- **tunnel**: Read-merge-write pattern to fix race conditions

## [0.22.2] - 2026-01-12

### Fixed
- **tunnel**: Sync warp-routing configuration to Cloudflare Remote Config

## [0.22.1] - 2026-01-12

### Fixed
- **access**: Include main domain in destinations for API validation
- **accessapplication**: Add Ingress/Tunnel watch and smart retry mechanism

## [0.22.0] - 2026-01-11

### Added
- **Six-Layer Unified Sync Architecture** implementation
  - Layer 1: K8s Resources (user-facing CRDs)
  - Layer 2: Resource Controllers (lightweight, validation)
  - Layer 3: Core Services (business logic)
  - Layer 4: CloudflareSyncState CRD (shared state with optimistic locking)
  - Layer 5: Sync Controllers (debouncing, aggregation, API calls)
  - Layer 6: Cloudflare API Client
- Eliminated race conditions through single sync point design
- 500ms debouncing for API call aggregation
- Hash-based change detection to reduce unnecessary API calls

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

## Version Summary

### CRD Count by Version

| Version | Total CRDs | New CRDs |
|---------|------------|----------|
| v0.27.x | 34 | - |
| v0.26.0 | 34 | PagesProject, PagesDomain, PagesDeployment |
| v0.25.0 | 31 | AccessPolicy |
| v0.24.0 | 30 | - (architecture improvements) |
| v0.22.0 | 30 | CloudflareSyncState |
| v0.21.0 | 29 | - (type safety) |
| v0.20.0 | 29 | R2Bucket, R2BucketDomain, R2BucketNotification, ZoneRuleset, TransformRule, RedirectRule, OriginCACertificate, DomainRegistration |
| v0.19.0 | 21 | CloudflareDomain |
| v0.18.0 | 20 | TunnelIngressClassConfig, TunnelGatewayClassConfig |

### Architecture Milestones

- **v0.22.0**: Six-Layer Unified Sync Architecture
- **v0.24.0**: L5 Sync Controller completion
- **v0.26.0**: Cloudflare Pages support
- **v0.27.0**: Inline Access policies

---

[Unreleased]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.6...HEAD
[0.27.6]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.5...v0.27.6
[0.27.5]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.4...v0.27.5
[0.27.4]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.3...v0.27.4
[0.27.3]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.2...v0.27.3
[0.27.2]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.1...v0.27.2
[0.27.1]: https://github.com/StringKe/cloudflare-operator/compare/v0.27.0...v0.27.1
[0.27.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.26.0...v0.27.0
[0.26.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.23.2...v0.24.0
[0.23.2]: https://github.com/StringKe/cloudflare-operator/compare/v0.23.1...v0.23.2
[0.23.1]: https://github.com/StringKe/cloudflare-operator/compare/v0.23.0...v0.23.1
[0.23.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.22.3...v0.23.0
[0.22.3]: https://github.com/StringKe/cloudflare-operator/compare/v0.22.2...v0.22.3
[0.22.2]: https://github.com/StringKe/cloudflare-operator/compare/v0.22.1...v0.22.2
[0.22.1]: https://github.com/StringKe/cloudflare-operator/compare/v0.22.0...v0.22.1
[0.22.0]: https://github.com/StringKe/cloudflare-operator/compare/v0.21.0...v0.22.0
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
