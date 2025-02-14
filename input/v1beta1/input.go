// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=github-app-get-token.fn.crossplane.giantswarm.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Input can be used to provide input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	SecretKey  string `json:"secretKey"`
	ContextKey string `json:"contextKey,omitempty"`
}
