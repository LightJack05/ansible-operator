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

// AnsibleHostSpec defines the desired state of AnsibleHost
type AnsibleHostSpec struct {
	// ansibleName is the name used in the Ansible inventory. Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_.-]+$`
	// +kubebuilder:validation:MaxLength=253
	AnsibleName string `json:"ansibleName,omitempty"`

	// connection defines SSH connection parameters for this host.
	// +required
	Connection AnsibleHostConnection `json:"connection"`

	// ssh defines SSH authentication configuration for this host.
	// +required
	SSH AnsibleHostSSH `json:"ssh"`

	// privilege defines privilege escalation settings.
	// +optional
	Privilege *AnsibleHostPrivilege `json:"privilege,omitempty"`

	// hostVars is an arbitrary string inserted verbatim into the Ansible host_vars block.
	// +optional
	HostVars string `json:"hostVars,omitempty"`
}

// AnsibleHostConnection defines SSH connection parameters.
type AnsibleHostConnection struct {
	// host is the hostname or IP address to connect to.
	// +required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`

	// port is the SSH port number.
	// +optional
	// +kubebuilder:default=22
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// user is the SSH login user.
	// +optional
	// +kubebuilder:default="root"
	// +kubebuilder:validation:Pattern=`^[a-z_][a-z0-9_-]*$`
	// +kubebuilder:validation:MaxLength=32
	User string `json:"user,omitempty"`
}

// AnsibleHostSSH defines SSH authentication configuration.
type AnsibleHostSSH struct {
	// ignoreHostKey disables SSH host key verification when true.
	// When false, the operator trusts the host key on first connection and stores it
	// in a secret. The auto-created secret is named after the AnsibleHost resource
	// unless sshHostKeySecretRef is provided.
	// +optional
	// +kubebuilder:default=false
	IgnoreHostKey bool `json:"ignoreHostKey,omitempty"`

	// sshKeySecretRef references a Secret containing the SSH private key.
	// The secret must have the key `ansible_ssh_private_key_file`.
	// +required
	SSHKeySecretRef corev1.LocalObjectReference `json:"sshKeySecretRef"`

	// sshHostKeySecretRef references a Secret used to store (or provide) the trusted SSH host key.
	// When ignoreHostKey is false and this is unset, the operator creates a secret automatically.
	// When set, the operator uses the referenced secret as the known-hosts store.
	// +required
	SSHHostKeySecretRef corev1.LocalObjectReference `json:"sshHostKeySecretRef,omitempty"`
}

// AnsibleHostPrivilege defines privilege escalation settings.
type AnsibleHostPrivilege struct {
	// become enables privilege escalation via sudo.
	// +optional
	// +kubebuilder:default=false
	Become bool `json:"become,omitempty"`
}

// AnsibleHostStatus defines the observed state of AnsibleHost.
type AnsibleHostStatus struct {
	// conditions represent the current state of the AnsibleHost resource.
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

// AnsibleHost is the Schema for the ansiblehosts API
type AnsibleHost struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AnsibleHost
	// +required
	Spec AnsibleHostSpec `json:"spec"`

	// status defines the observed state of AnsibleHost
	// +optional
	Status AnsibleHostStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AnsibleHostList contains a list of AnsibleHost
type AnsibleHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AnsibleHost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AnsibleHost{}, &AnsibleHostList{})
}
