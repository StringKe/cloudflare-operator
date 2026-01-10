// SPDX-License-Identifier: Apache-2.0
// Copyright 2025-2026 The Cloudflare Operator Authors

package v1alpha1

import (
	"log"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/StringKe/cloudflare-operator/api/v1alpha2"
)

// ConvertTo converts this ClusterTunnel (v1alpha1) to the Hub version (v1alpha2).
func (src *ClusterTunnel) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.ClusterTunnel)
	log.Printf("ConvertTo: Converting ClusterTunnel from Spoke version v1alpha1 to Hub version v1alpha2;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	// Implement conversion logic from v1alpha1 to v1alpha2
	dst.ObjectMeta = src.ObjectMeta
	if err := src.Spec.ConvertTo(&dst.Spec); err != nil {
		return err
	}
	if err := src.Status.ConvertTo(&dst.Status); err != nil {
		return err
	}
	return nil
}

// ConvertFrom converts the Hub version (v1alpha2) to this ClusterTunnel (v1alpha1).
func (dst *ClusterTunnel) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.ClusterTunnel)
	log.Printf("ConvertFrom: Converting ClusterTunnel from Hub version v1alpha2 to Spoke version v1alpha1;"+
		"source: %s/%s, target: %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)

	// Implement conversion logic from v1alpha2 to v1alpha1
	dst.ObjectMeta = src.ObjectMeta
	if err := dst.Spec.ConvertFrom(src.Spec); err != nil {
		return err
	}
	if err := dst.Status.ConvertFrom(src.Status); err != nil {
		return err
	}
	return nil
}
