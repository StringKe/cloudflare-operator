#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2025-2026 The Cloudflare Operator Authors
#
# TunnelBinding -> Ingress Migration Tool
#
# This script helps migrate TunnelBinding resources to Kubernetes Ingress
# resources with TunnelIngressClassConfig.
#
# Usage: ./migrate-tunnelbinding.sh [namespace] [output-dir]
#   namespace: Kubernetes namespace to scan (default: default)
#   output-dir: Directory to write migration files (default: ./migration-output)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${1:-default}"
OUTPUT_DIR="${2:-./migration-output}"
DRY_RUN="${DRY_RUN:-true}"

echo -e "${BLUE}=== TunnelBinding Migration Tool ===${NC}"
echo -e "Scanning namespace: ${YELLOW}$NAMESPACE${NC}"
echo -e "Output directory: ${YELLOW}$OUTPUT_DIR${NC}"
echo ""

# Check prerequisites
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl is not installed${NC}"
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: jq is not installed${NC}"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Counter
TOTAL_BINDINGS=0
MIGRATED_BINDINGS=0

# Get all TunnelBindings in namespace
BINDINGS=$(kubectl get tunnelbinding -n "$NAMESPACE" -o json 2>/dev/null || echo '{"items":[]}')
BINDING_COUNT=$(echo "$BINDINGS" | jq '.items | length')

if [ "$BINDING_COUNT" -eq 0 ]; then
    echo -e "${YELLOW}No TunnelBinding resources found in namespace $NAMESPACE${NC}"
    echo ""
    echo "If TunnelBindings exist in other namespaces, run:"
    echo "  $0 <namespace>"
    exit 0
fi

echo -e "Found ${GREEN}$BINDING_COUNT${NC} TunnelBinding(s)"
echo ""

# Process each TunnelBinding
echo "$BINDINGS" | jq -c '.items[]' | while read -r binding; do
    TOTAL_BINDINGS=$((TOTAL_BINDINGS + 1))

    NAME=$(echo "$binding" | jq -r '.metadata.name')
    BINDING_NS=$(echo "$binding" | jq -r '.metadata.namespace')

    echo -e "${BLUE}Processing: ${NC}$BINDING_NS/$NAME"

    # Extract tunnel reference
    TUNNEL_KIND=$(echo "$binding" | jq -r '.tunnelRef.kind')
    TUNNEL_NAME=$(echo "$binding" | jq -r '.tunnelRef.name')
    DISABLE_DNS=$(echo "$binding" | jq -r '.tunnelRef.disableDNSUpdates // false')

    # Generate IngressClass name based on tunnel
    if [ "$TUNNEL_KIND" = "ClusterTunnel" ]; then
        INGRESS_CLASS="tunnel-${TUNNEL_NAME}"
    else
        INGRESS_CLASS="tunnel-${BINDING_NS}-${TUNNEL_NAME}"
    fi

    echo -e "  Tunnel Reference: ${TUNNEL_KIND}/${TUNNEL_NAME}"
    echo -e "  IngressClass: ${INGRESS_CLASS}"

    # Create TunnelIngressClassConfig if not exists
    CONFIG_FILE="$OUTPUT_DIR/00-ingressclassconfig-${TUNNEL_NAME}.yaml"
    if [ ! -f "$CONFIG_FILE" ]; then
        cat > "$CONFIG_FILE" << EOF
# TunnelIngressClassConfig for ${TUNNEL_KIND}/${TUNNEL_NAME}
# Auto-generated from TunnelBinding migration
apiVersion: networking.cloudflare-operator.io/v1alpha2
kind: TunnelIngressClassConfig
metadata:
  name: ${INGRESS_CLASS}
spec:
  tunnelRef:
    kind: ${TUNNEL_KIND}
    name: ${TUNNEL_NAME}
  # Adjust DNS management mode as needed:
  # - Automatic: DNS records managed by operator
  # - DNSRecord: Creates DNSRecord CRDs
  # - Manual: DNS records managed externally
  dnsManagement: Automatic
  dnsProxied: true
EOF
        echo -e "  ${GREEN}Created:${NC} $CONFIG_FILE"
    fi

    # Create IngressClass if not exists
    INGRESS_CLASS_FILE="$OUTPUT_DIR/00-ingressclass-${INGRESS_CLASS}.yaml"
    if [ ! -f "$INGRESS_CLASS_FILE" ]; then
        cat > "$INGRESS_CLASS_FILE" << EOF
# IngressClass for ${INGRESS_CLASS}
# Auto-generated from TunnelBinding migration
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: ${INGRESS_CLASS}
  annotations:
    migrated-from: tunnelbinding
spec:
  controller: cloudflare-operator.io/tunnel-ingress-controller
  parameters:
    apiGroup: networking.cloudflare-operator.io
    kind: TunnelIngressClassConfig
    name: ${INGRESS_CLASS}
EOF
        echo -e "  ${GREEN}Created:${NC} $INGRESS_CLASS_FILE"
    fi

    # Generate Ingress for each subject
    echo "$binding" | jq -c '.subjects[]' | while read -r subject; do
        SERVICE_NAME=$(echo "$subject" | jq -r '.name')
        FQDN=$(echo "$subject" | jq -r '.spec.fqdn // empty')
        PROTOCOL=$(echo "$subject" | jq -r '.spec.protocol // "http"')
        PATH_REGEX=$(echo "$subject" | jq -r '.spec.path // empty')
        TARGET=$(echo "$subject" | jq -r '.spec.target // empty')
        NO_TLS_VERIFY=$(echo "$subject" | jq -r '.spec.noTlsVerify // false')
        HTTP2_ORIGIN=$(echo "$subject" | jq -r '.spec.http2Origin // false')
        CA_POOL=$(echo "$subject" | jq -r '.spec.caPool // empty')

        # Skip if no FQDN
        if [ -z "$FQDN" ]; then
            echo -e "  ${YELLOW}Warning:${NC} Subject $SERVICE_NAME has no FQDN, skipping"
            continue
        fi

        INGRESS_NAME="${NAME}-${SERVICE_NAME}"
        INGRESS_FILE="$OUTPUT_DIR/${NAME}-${SERVICE_NAME}-ingress.yaml"

        # Build annotations
        ANNOTATIONS=""
        if [ "$PROTOCOL" = "https" ]; then
            ANNOTATIONS="${ANNOTATIONS}    cloudflare-operator.io/origin-request-no-tls-verify: \"${NO_TLS_VERIFY}\"\n"
        fi
        if [ "$HTTP2_ORIGIN" = "true" ]; then
            ANNOTATIONS="${ANNOTATIONS}    cloudflare-operator.io/origin-request-http2-origin: \"true\"\n"
        fi
        if [ -n "$CA_POOL" ]; then
            ANNOTATIONS="${ANNOTATIONS}    cloudflare-operator.io/origin-request-ca-pool: \"${CA_POOL}\"\n"
        fi
        if [ -n "$TARGET" ]; then
            ANNOTATIONS="${ANNOTATIONS}    # Original target: ${TARGET}\n"
        fi

        # Determine path
        INGRESS_PATH="/"
        PATH_TYPE="Prefix"
        if [ -n "$PATH_REGEX" ]; then
            INGRESS_PATH="$PATH_REGEX"
            PATH_TYPE="ImplementationSpecific"
        fi

        # Create Ingress
        cat > "$INGRESS_FILE" << EOF
# Ingress migrated from TunnelBinding: ${NAME}
# Subject: ${SERVICE_NAME}
# Original FQDN: ${FQDN}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${INGRESS_NAME}
  namespace: ${BINDING_NS}
  labels:
    migrated-from: tunnelbinding/${NAME}
  annotations:
    cloudflare-operator.io/origin-request-protocol: "${PROTOCOL}"
$(echo -e "$ANNOTATIONS")
spec:
  ingressClassName: ${INGRESS_CLASS}
  rules:
  - host: ${FQDN}
    http:
      paths:
      - path: ${INGRESS_PATH}
        pathType: ${PATH_TYPE}
        backend:
          service:
            name: ${SERVICE_NAME}
            port:
              number: 80  # FIXME: Update to actual service port
EOF
        echo -e "  ${GREEN}Created:${NC} $INGRESS_FILE"
    done

    MIGRATED_BINDINGS=$((MIGRATED_BINDINGS + 1))
done

echo ""
echo -e "${GREEN}=== Migration Complete ===${NC}"
echo ""
echo -e "Generated files in: ${YELLOW}$OUTPUT_DIR${NC}"
echo ""
echo -e "${BLUE}Next Steps:${NC}"
echo ""
echo "1. Review the generated files in $OUTPUT_DIR"
echo "   - Check Ingress service port numbers (set to 80 as placeholder)"
echo "   - Verify TunnelIngressClassConfig DNS settings"
echo "   - Review origin-request annotations"
echo ""
echo "2. Ensure TunnelIngressClassConfig CRD is available:"
echo "   kubectl get crd tunnelingressclassconfigs.networking.cloudflare-operator.io"
echo ""
echo "3. Apply the migration files in order:"
echo "   kubectl apply -f $OUTPUT_DIR/00-ingressclassconfig-*.yaml"
echo "   kubectl apply -f $OUTPUT_DIR/00-ingressclass-*.yaml"
echo "   kubectl apply -f $OUTPUT_DIR/*-ingress.yaml"
echo ""
echo "4. Verify the Ingress resources are working:"
echo "   kubectl get ingress -n $NAMESPACE"
echo "   kubectl describe ingress -n $NAMESPACE"
echo ""
echo "5. Once verified, delete the TunnelBinding resources:"
echo -e "   ${YELLOW}WARNING: This will remove DNS records managed by TunnelBinding!${NC}"
echo "   kubectl delete tunnelbinding -n $NAMESPACE <name>"
echo ""
echo "6. For detailed migration documentation, see:"
echo "   docs/en/migration/tunnelbinding-migration.md"
echo ""
