/*
Copyright 2025 Adyanth H.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	// Name is the DNS record name (e.g., "www" or "www.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type is the DNS record type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;TXT;MX;NS;SRV;CAA;CERT;DNSKEY;DS;HTTPS;LOC;NAPTR;SMIMEA;SSHFP;SVCB;TLSA;URI
	Type string `json:"type"`

	// Content is the record content/value.
	// +kubebuilder:validation:Required
	Content string `json:"content"`

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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dnsrec
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Content",type=string,JSONPath=`.spec.content`
// +kubebuilder:printcolumn:name="Proxied",type=boolean,JSONPath=`.spec.proxied`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
