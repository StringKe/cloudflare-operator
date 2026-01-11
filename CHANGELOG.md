# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- New CRDs for R2 Storage: R2Bucket, R2BucketDomain, R2BucketNotification
- New CRDs for Rules Engine: ZoneRuleset, TransformRule, RedirectRule
- New CRD for SSL/TLS: OriginCACertificate
- New CRD for Registrar: DomainRegistration (Enterprise)
- Improved GitHub Actions security with pinned SHA hashes
- Minimal token permissions per workflow job

### Changed
- Updated SECURITY.md to support v0.18.x and v0.19.x

## [0.19.5] - 2026-01-10

### Fixed
- Fixed CI/CD workflow issues

## [0.19.4] - 2026-01-10

### Fixed
- Fixed CI/CD workflow issues

## [0.19.3] - 2026-01-10

### Fixed
- Fixed CI/CD workflow issues

## [0.19.2] - 2026-01-10

### Fixed
- Fixed DNS record creation with trailing dots in domain validation
- Fixed DNSRecord controller to respect spec.cloudflare.domain field for zone resolution
- Restored correct tool versions in Makefile

### Added
- Added domain validation to prevent wrong zone record creation

## [0.19.1] - 2026-01-10

### Fixed
- Fixed race condition in CloudflareDomain state changes
- Added CRD creation checklist to documentation

## [0.19.0] - 2026-01-10

### Added
- New CloudflareDomain CRD for multi-zone DNS support
- Multi-zone support for DNSRecord resources

## [0.18.5] - 2026-01-10

### Fixed
- Fixed Ingress controller to set ValidTunnelId in API client for DNS operations

## [0.18.4] - 2026-01-10

### Fixed
- Fixed Ingress controller JSON tags for Configuration structs (sigs.k8s.io/yaml compatibility)

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
- TunnelIngressClassConfig CRD for Kubernetes Ingress integration
- TunnelGatewayClassConfig CRD for Gateway API integration
- Native Kubernetes Ingress controller support
- Gateway API support (Gateway, HTTPRoute, TCPRoute, UDPRoute)

### Changed
- Major refactoring of tunnel configuration management
- Improved route building and path handling

## [0.17.x] and earlier

See [GitHub Releases](https://github.com/StringKe/cloudflare-operator/releases) for historical changes.

[Unreleased]: https://github.com/StringKe/cloudflare-operator/compare/v0.19.5...HEAD
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
