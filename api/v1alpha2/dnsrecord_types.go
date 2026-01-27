// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceAddressType defines how to extract address from a Service.
// +kubebuilder:validation:Enum=LoadBalancerIP;LoadBalancerHostname;ExternalIP;ExternalName;ClusterIP
type ServiceAddressType string

const (
	// ServiceAddressLoadBalancerIP uses .status.loadBalancer.ingress[].ip
	ServiceAddressLoadBalancerIP ServiceAddressType = "LoadBalancerIP"
	// ServiceAddressLoadBalancerHostname uses .status.loadBalancer.ingress[].hostname
	ServiceAddressLoadBalancerHostname ServiceAddressType = "LoadBalancerHostname"
	// ServiceAddressExternalIP uses .spec.externalIPs[]
	ServiceAddressExternalIP ServiceAddressType = "ExternalIP"
	// ServiceAddressExternalName uses .spec.externalName (for ExternalName type services)
	ServiceAddressExternalName ServiceAddressType = "ExternalName"
	// ServiceAddressClusterIP uses .spec.clusterIP
	ServiceAddressClusterIP ServiceAddressType = "ClusterIP"
)

// NodeAddressType defines how to extract address from a Node.
// +kubebuilder:validation:Enum=InternalIP;ExternalIP;Hostname
type NodeAddressType string

const (
	// NodeAddressInternalIP uses .status.addresses[type=InternalIP].address
	NodeAddressInternalIP NodeAddressType = "InternalIP"
	// NodeAddressExternalIP uses .status.addresses[type=ExternalIP].address
	NodeAddressExternalIP NodeAddressType = "ExternalIP"
	// NodeAddressHostname uses .status.addresses[type=Hostname].address
	NodeAddressHostname NodeAddressType = "Hostname"
)

// AddressSelectionPolicy defines how to select addresses when multiple are available.
// +kubebuilder:validation:Enum=First;All;PreferIPv4;PreferIPv6
type AddressSelectionPolicy string

const (
	// AddressSelectionFirst uses the first available address.
	AddressSelectionFirst AddressSelectionPolicy = "First"
	// AddressSelectionAll creates multiple DNS records (round-robin).
	AddressSelectionAll AddressSelectionPolicy = "All"
	// AddressSelectionPreferIPv4 prefers IPv4 addresses.
	AddressSelectionPreferIPv4 AddressSelectionPolicy = "PreferIPv4"
	// AddressSelectionPreferIPv6 prefers IPv6 addresses.
	AddressSelectionPreferIPv6 AddressSelectionPolicy = "PreferIPv6"
)

// SourceDeletionPolicy defines behavior when the source resource is deleted.
// +kubebuilder:validation:Enum=Delete;Orphan
type SourceDeletionPolicy string

const (
	// SourceDeletionDelete deletes the DNS record when the source is deleted.
	SourceDeletionDelete SourceDeletionPolicy = "Delete"
	// SourceDeletionOrphan keeps the DNS record when the source is deleted.
	SourceDeletionOrphan SourceDeletionPolicy = "Orphan"
)

// ServiceDNSSource extracts address from a Kubernetes Service.
type ServiceDNSSource struct {
	// Name is the name of the Service.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Service.
	// Defaults to the namespace of the DNSRecord.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// AddressType specifies which address to extract from the Service.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=LoadBalancerIP
	AddressType ServiceAddressType `json:"addressType,omitempty"`
}

// IngressDNSSource extracts address from a Kubernetes Ingress.
type IngressDNSSource struct {
	// Name is the name of the Ingress.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Ingress.
	// Defaults to the namespace of the DNSRecord.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// HTTPRouteDNSSource extracts address from a Gateway API HTTPRoute.
type HTTPRouteDNSSource struct {
	// Name is the name of the HTTPRoute.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the HTTPRoute.
	// Defaults to the namespace of the DNSRecord.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// GatewayDNSSource extracts address from a Gateway API Gateway.
type GatewayDNSSource struct {
	// Name is the name of the Gateway.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Gateway.
	// Defaults to the namespace of the DNSRecord.
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// NodeDNSSource extracts address from a Kubernetes Node.
type NodeDNSSource struct {
	// Name is the name of the Node.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// AddressType specifies which address to extract from the Node.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ExternalIP
	AddressType NodeAddressType `json:"addressType,omitempty"`
}

// DNSRecordSourceRef defines a source to dynamically obtain DNS record content.
// Exactly one source type must be specified.
type DNSRecordSourceRef struct {
	// Service extracts address from a Kubernetes Service.
	// +kubebuilder:validation:Optional
	Service *ServiceDNSSource `json:"service,omitempty"`

	// Ingress extracts address from a Kubernetes Ingress.
	// +kubebuilder:validation:Optional
	Ingress *IngressDNSSource `json:"ingress,omitempty"`

	// HTTPRoute extracts address from a Gateway API HTTPRoute.
	// +kubebuilder:validation:Optional
	HTTPRoute *HTTPRouteDNSSource `json:"httpRoute,omitempty"`

	// Gateway extracts address from a Gateway API Gateway.
	// +kubebuilder:validation:Optional
	Gateway *GatewayDNSSource `json:"gateway,omitempty"`

	// Node extracts address from a Kubernetes Node.
	// +kubebuilder:validation:Optional
	Node *NodeDNSSource `json:"node,omitempty"`
}

// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	// Name is the DNS record name (e.g., "www" or "www.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type is the DNS record type.
	// Optional when using sourceRef - will be auto-detected based on address type.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;TXT;MX;NS;SRV;CAA;CERT;DNSKEY;DS;HTTPS;LOC;NAPTR;SMIMEA;SSHFP;SVCB;TLSA;URI
	Type string `json:"type,omitempty"`

	// Content is the static record content/value.
	// Mutually exclusive with sourceRef.
	// +kubebuilder:validation:Optional
	Content string `json:"content,omitempty"`

	// SourceRef dynamically obtains the record content from a Kubernetes resource.
	// Mutually exclusive with content.
	// +kubebuilder:validation:Optional
	SourceRef *DNSRecordSourceRef `json:"sourceRef,omitempty"`

	// AddressSelection specifies how to select addresses when multiple are available.
	// Only applies when using sourceRef.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=First
	AddressSelection AddressSelectionPolicy `json:"addressSelection,omitempty"`

	// SourceDeletionPolicy specifies behavior when the source resource is deleted.
	// Only applies when using sourceRef.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Delete
	SourceDeletionPolicy SourceDeletionPolicy `json:"sourceDeletionPolicy,omitempty"`

	// TTL is the Time To Live (1 = automatic).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	TTL int `json:"ttl,omitempty"`

	// Proxied enables Cloudflare proxy for this record.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Proxied bool `json:"proxied,omitempty"`

	// Priority for MX/SRV records.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	Priority *int `json:"priority,omitempty"`

	// Comment is an optional comment.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=100
	Comment string `json:"comment,omitempty"`

	// Tags for the record.
	// +kubebuilder:validation:Optional
	Tags []string `json:"tags,omitempty"`

	// Data contains additional record-specific data (for SRV, CAA, etc.).
	// +kubebuilder:validation:Optional
	Data *DNSRecordData `json:"data,omitempty"`

	// Cloudflare contains the Cloudflare API credentials.
	// +kubebuilder:validation:Required
	Cloudflare CloudflareDetails `json:"cloudflare"`
}

// DNSRecordData contains type-specific record data.
type DNSRecordData struct {
	// For SRV records
	// +kubebuilder:validation:Optional
	Service string `json:"service,omitempty"`

	// +kubebuilder:validation:Optional
	Proto string `json:"proto,omitempty"`

	// +kubebuilder:validation:Optional
	Weight int `json:"weight,omitempty"`

	// +kubebuilder:validation:Optional
	Port int `json:"port,omitempty"`

	// +kubebuilder:validation:Optional
	Target string `json:"target,omitempty"`

	// For CAA records
	// +kubebuilder:validation:Optional
	Flags int `json:"flags,omitempty"`

	// +kubebuilder:validation:Optional
	Tag string `json:"tag,omitempty"`

	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`

	// For CERT/SSHFP/TLSA records
	// +kubebuilder:validation:Optional
	Algorithm int `json:"algorithm,omitempty"`

	// +kubebuilder:validation:Optional
	Certificate string `json:"certificate,omitempty"`

	// +kubebuilder:validation:Optional
	KeyTag int `json:"keyTag,omitempty"`

	// +kubebuilder:validation:Optional
	Usage int `json:"usage,omitempty"`

	// +kubebuilder:validation:Optional
	Selector int `json:"selector,omitempty"`

	// +kubebuilder:validation:Optional
	MatchingType int `json:"matchingType,omitempty"`

	// For LOC records
	// +kubebuilder:validation:Optional
	LatDegrees int `json:"latDegrees,omitempty"`

	// +kubebuilder:validation:Optional
	LatMinutes int `json:"latMinutes,omitempty"`

	// +kubebuilder:validation:Optional
	LatSeconds string `json:"latSeconds,omitempty"`

	// +kubebuilder:validation:Optional
	LatDirection string `json:"latDirection,omitempty"`

	// +kubebuilder:validation:Optional
	LongDegrees int `json:"longDegrees,omitempty"`

	// +kubebuilder:validation:Optional
	LongMinutes int `json:"longMinutes,omitempty"`

	// +kubebuilder:validation:Optional
	LongSeconds string `json:"longSeconds,omitempty"`

	// +kubebuilder:validation:Optional
	LongDirection string `json:"longDirection,omitempty"`

	// +kubebuilder:validation:Optional
	Altitude string `json:"altitude,omitempty"`

	// +kubebuilder:validation:Optional
	Size string `json:"size,omitempty"`

	// +kubebuilder:validation:Optional
	PrecisionHorz string `json:"precisionHorz,omitempty"`

	// +kubebuilder:validation:Optional
	PrecisionVert string `json:"precisionVert,omitempty"`

	// For URI records
	// +kubebuilder:validation:Optional
	ContentURI string `json:"content,omitempty"`
}

// DNSRecordStatus defines the observed state
type DNSRecordStatus struct {
	// RecordID is the Cloudflare DNS Record ID.
	// +kubebuilder:validation:Optional
	RecordID string `json:"recordId,omitempty"`

	// ZoneID is the Cloudflare Zone ID.
	// +kubebuilder:validation:Optional
	ZoneID string `json:"zoneId,omitempty"`

	// FQDN is the fully qualified domain name.
	// +kubebuilder:validation:Optional
	FQDN string `json:"fqdn,omitempty"`

	// State indicates the current state.
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`

	// Conditions represent the latest available observations.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ResolvedType is the auto-detected record type when using sourceRef.
	// +kubebuilder:validation:Optional
	ResolvedType string `json:"resolvedType,omitempty"`

	// ResolvedContent is the content resolved from the source resource.
	// +kubebuilder:validation:Optional
	ResolvedContent string `json:"resolvedContent,omitempty"`

	// ResolvedAddresses contains all addresses resolved from the source resource.
	// Populated when using AddressSelection=All.
	// +kubebuilder:validation:Optional
	ResolvedAddresses []string `json:"resolvedAddresses,omitempty"`

	// SourceResourceVersion is the resourceVersion of the source resource.
	// Used to detect changes in the source.
	// +kubebuilder:validation:Optional
	SourceResourceVersion string `json:"sourceResourceVersion,omitempty"`

	// ManagedRecordIDs contains all DNS record IDs managed by this resource.
	// Multiple IDs when using AddressSelection=All.
	// +kubebuilder:validation:Optional
	ManagedRecordIDs []string `json:"managedRecordIds,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dnsrec
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`,priority=0
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`,priority=0
// +kubebuilder:printcolumn:name="Content",type=string,JSONPath=`.spec.content`,priority=0
// +kubebuilder:printcolumn:name="Resolved",type=string,JSONPath=`.status.resolvedContent`,priority=1
// +kubebuilder:printcolumn:name="Proxied",type=boolean,JSONPath=`.spec.proxied`,priority=0
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0

// DNSRecord is the Schema for the dnsrecords API.
type DNSRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSRecordSpec   `json:"spec,omitempty"`
	Status DNSRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DNSRecordList contains a list of DNSRecord
type DNSRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSRecord{}, &DNSRecordList{})
}

// IsDynamicMode returns true if the DNSRecord uses sourceRef for dynamic content.
func (r *DNSRecord) IsDynamicMode() bool {
	return r.Spec.SourceRef != nil
}

// IsStaticMode returns true if the DNSRecord uses static content.
func (r *DNSRecord) IsStaticMode() bool {
	return r.Spec.Content != "" && r.Spec.SourceRef == nil
}

// GetSourceType returns the type of source being used, or empty string if static mode.
func (s *DNSRecordSourceRef) GetSourceType() string {
	if s == nil {
		return ""
	}
	if s.Service != nil {
		return "Service"
	}
	if s.Ingress != nil {
		return "Ingress"
	}
	if s.HTTPRoute != nil {
		return "HTTPRoute"
	}
	if s.Gateway != nil {
		return "Gateway"
	}
	if s.Node != nil {
		return "Node"
	}
	return ""
}

// CountSources returns the number of source types specified.
func (s *DNSRecordSourceRef) CountSources() int {
	if s == nil {
		return 0
	}
	count := 0
	if s.Service != nil {
		count++
	}
	if s.Ingress != nil {
		count++
	}
	if s.HTTPRoute != nil {
		count++
	}
	if s.Gateway != nil {
		count++
	}
	if s.Node != nil {
		count++
	}
	return count
}
