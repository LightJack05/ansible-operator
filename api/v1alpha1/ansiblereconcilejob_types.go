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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AnsibleReconcileJobSpec defines the desired state of AnsibleReconcileJob
type AnsibleReconcileJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// TODO: This field needs better validation in the future! K8s does synchronous validation by parsing the schedule using a library, we may want to replicate that using a webhook...
	// +kubebuilder:validation:Required
	// Schedule is a cron expression that defines when the AnsibleReconcileJob should run.
	Schedule string `json:"schedule,omitempty"`
	// PlaybookRef is a reference to the AnsiblePlaybook resource that should be executed by this AnsibleReconcileJob.
	// +kubebuilder:validation:Required
	PlaybookRef corev1.LocalObjectReference `json:"playbookRef,omitempty"`
}

// AnsibleReconcileJobStatus defines the observed state of AnsibleReconcileJob.
type AnsibleReconcileJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the AnsibleReconcileJob resource.
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

// AnsibleReconcileJob is the Schema for the ansiblereconcilejobs API
type AnsibleReconcileJob struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AnsibleReconcileJob
	// +required
	Spec AnsibleReconcileJobSpec `json:"spec"`

	// status defines the observed state of AnsibleReconcileJob
	// +optional
	Status AnsibleReconcileJobStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AnsibleReconcileJobList contains a list of AnsibleReconcileJob
type AnsibleReconcileJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AnsibleReconcileJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AnsibleReconcileJob{}, &AnsibleReconcileJobList{})
}
