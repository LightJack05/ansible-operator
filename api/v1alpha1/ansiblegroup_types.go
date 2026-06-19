/*
Copyright 2026.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AnsibleGroupConditionReady           = "Ready"
	AnsibleGroupConditionHealthy         = "Healthy"
	AnsibleGroupConditionReferencesValid = "ReferencesValid"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AnsibleGroupSpec defines the desired state of AnsibleGroup
type AnsibleGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// AnsibleName is the name of the object within the Ansible inventory
	// +required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_.-]+$`
	// +kubebuilder:validation:MaxLength=253
	AnsibleName string `json:"ansibleName"`

	// Hosts is a list of references to AnsibleHost objects that are members of this group.
	// +required
	Hosts []corev1.LocalObjectReference `json:"hosts"`

	// Groups is a list of references to other AnsibleGroup objects that are subgroups of this group.
	// +required
	Groups []corev1.LocalObjectReference `json:"groups"`

	// TODO: Be wary of injection on this one, just like with hostVars.
	// groupVars is an arbitrary string inserted verbatim into the Ansible group_vars file for this group.
	// +optional
	GroupVars string `json:"groupVars,omitempty"`
}

// AnsibleGroupStatus defines the observed state of AnsibleGroup.
type AnsibleGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the AnsibleGroup resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AnsibleGroup is the Schema for the ansiblegroups API
type AnsibleGroup struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AnsibleGroup
	// +required
	Spec AnsibleGroupSpec `json:"spec"`

	// status defines the observed state of AnsibleGroup
	// +optional
	Status AnsibleGroupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AnsibleGroupList contains a list of AnsibleGroup
type AnsibleGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AnsibleGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AnsibleGroup{}, &AnsibleGroupList{})
}
