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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	anisbleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
	"github.com/LightJack05/ansible-operator/internal/ssh"
)

// AnsibleHostReconciler reconciles a AnsibleHost object
type AnsibleHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=anisble-operator.lightjack.de,resources=ansiblehosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=anisble-operator.lightjack.de,resources=ansiblehosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=anisble-operator.lightjack.de,resources=ansiblehosts/finalizers,verbs=update
// +kubebuilder:rbac:groups=anisble-operator.lightjack.de,resources=ansiblehosts/finalizers,verbs=update
// Access for reading and writing the SSH keys
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AnsibleHost object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AnsibleHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := logf.FromContext(ctx)
	lg.Info("Reconciling AnsibleHost", "namespace", req.Namespace, "name", req.Name)

	// Get the AnsibleHost resource
	var ansibleHost anisbleoperatorv1alpha1.AnsibleHost
	if err := r.Get(ctx, req.NamespacedName, &ansibleHost); err != nil {
		// If the resource is not found, it might have been deleted after the reconcile request was queued.
		// In this case, we can ignore the error and return.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the referenced SSH key secret exists and is valid
	if err := r.checkHostCredentialExists(ctx, &ansibleHost); err != nil {
		// If the secret does not exist or is invalid, we can log the error and requeue the request
		// to check again later.
		if err := r.setStatusNotReady(ctx, &ansibleHost, "SSHKeySecretInvalid", fmt.Sprintf("SSH key secret is missing or invalid: %v", err)); err != nil {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("failed to update AnsibleHost status: %w", err)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("SSH key secret is missing or invalid: %w", err)
	}

	// Ensure the host keys secret exists
	if err := r.ensureHostKeysSecretExists(ctx, &ansibleHost); err != nil {
		// If there was an error ensuring the host keys secret exists, we can log the error and requeue the request
		if err := r.setStatusNotReady(ctx, &ansibleHost, "HostKeysSecretError", fmt.Sprintf("Failed to ensure host keys secret exists: %v", err)); err != nil {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("failed to update AnsibleHost status: %w", err)
		}

		return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("failed to ensure host keys secret exists: %w", err)
	}

	// If we reach this point, the AnsibleHost is ready
	if err := r.setStatusReady(ctx, &ansibleHost); err != nil {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, fmt.Errorf("failed to update AnsibleHost status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AnsibleHostReconciler) setStatusReady(ctx context.Context, ansibleHost *anisbleoperatorv1alpha1.AnsibleHost) error {
	meta.SetStatusCondition(&ansibleHost.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "HostReady",
		Message: "The AnsibleHost is ready to be used",
	})
	err := r.Status().Update(ctx, ansibleHost)
	if err != nil {
		return fmt.Errorf("failed to update AnsibleHost status: %w", err)
	}
	return nil
}

func (r *AnsibleHostReconciler) setStatusNotReady(ctx context.Context, ansibleHost *anisbleoperatorv1alpha1.AnsibleHost, reason, message string) error {
	meta.SetStatusCondition(&ansibleHost.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	err := r.Status().Update(ctx, ansibleHost)
	if err != nil {
		return fmt.Errorf("failed to update AnsibleHost status: %w", err)
	}
	return nil
}

func (r *AnsibleHostReconciler) checkHostCredentialExists(ctx context.Context, ansibleHost *anisbleoperatorv1alpha1.AnsibleHost) error {
	// Check if the secret already exists
	var secret corev1.Secret
	err := r.Get(ctx, client.ObjectKey{Namespace: ansibleHost.Namespace, Name: ansibleHost.Spec.SSH.SSHKeySecretRef.Name}, &secret)
	if err != nil {
		return fmt.Errorf("secret %s not found in namespace %s: %w", ansibleHost.Spec.SSH.SSHKeySecretRef.Name, ansibleHost.ObjectMeta.Namespace, err)
	}

	// Secret already exists, validate it
	valid, err := r.secretHasValidSSHKey(&secret)
	if err != nil {
		return fmt.Errorf("failed to validate existing secret: %w", err)
	}
	if !valid {
		return fmt.Errorf("existing secret does not contain a valid SSH key")
	}
	return nil

}

func (r *AnsibleHostReconciler) secretHasValidSSHKey(secret *corev1.Secret) (bool, error) {
	var sshKey string
	const keyName = "ansible_ssh_private_key_file"
	if keyData, ok := secret.Data[keyName]; ok {
		sshKey = string(keyData)
	} else {
		return false, fmt.Errorf("secret does not contain '%s' field", keyName)
	}

	return ssh.ValidatePrivateSSHKey(sshKey)
}

func (r *AnsibleHostReconciler) ensureHostKeysSecretExists(ctx context.Context, ansibleHost *anisbleoperatorv1alpha1.AnsibleHost) error {
	// Check if the host keys secret already exists
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ansibleHost.Namespace, Name: ansibleHost.Spec.SSH.SSHHostKeySecretRef.Name}, secret)
	if err == nil {
		// Secret exists, validate it still matches the host keys
		valid, err := r.checkHostKeysSecret(ansibleHost, secret)
		if err != nil {
			return fmt.Errorf("failed to validate existing host keys secret: %w", err)
		}
		if !valid {
			//TODO: set a status here to indicate that the host keys have changed
			// 		DO NOT REISSUE, this will make the entire key exchange obsolete
			// 		Wait for the user to delete the secret and then recreate it with new keys.
		}
		return nil
	}
	if errors.IsNotFound(err) {
		// Secret does not exist, create it
		if err := r.createHostKeysSecret(ctx, ansibleHost); err != nil {
			return fmt.Errorf("failed to create host keys secret: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to get host keys secret: %w", err)

}

func (r *AnsibleHostReconciler) checkHostKeysSecret(ansibleHost *anisbleoperatorv1alpha1.AnsibleHost, secret *corev1.Secret) (bool, error) {
	expectedKeys, err := getHostKeys(ansibleHost)
	if err != nil {
		return false, fmt.Errorf("failed to get expected host keys: %w", err)
	}

	keyData, ok := secret.Data["host_keys"]
	if !ok {
		return false, fmt.Errorf("secret does not contain 'host_keys' field")
	}
	actualKeys := string(keyData)

	return expectedKeys == actualKeys, nil
}

func (r *AnsibleHostReconciler) createHostKeysSecret(ctx context.Context, ansibleHost *anisbleoperatorv1alpha1.AnsibleHost) error {
	hostKeys, err := getHostKeys(ansibleHost)
	if err != nil {
		return fmt.Errorf("failed to get host keys: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      ansibleHost.Spec.SSH.SSHHostKeySecretRef.Name,
			Namespace: ansibleHost.Namespace,
		},
		Data: map[string][]byte{
			"host_keys": []byte(hostKeys),
		},
	}

	if err := ctrl.SetControllerReference(ansibleHost, secret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.Create(ctx, secret); err != nil {
		return fmt.Errorf("failed to create host keys secret: %w", err)
	}
	return nil
}

func getHostKeys(ansibleHost *anisbleoperatorv1alpha1.AnsibleHost) (string, error) {
	keys, err := ssh.ScanHost(ansibleHost.Spec.Connection.Host, int(ansibleHost.Spec.Connection.Port))
	if err != nil {
		return "", fmt.Errorf("failed to scan host keys: %w", err)
	}

	keyString, err := ssh.HostKeysToString(keys)
	if err != nil {
		return "", fmt.Errorf("failed to convert host keys to string: %w", err)
	}

	return keyString, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&anisbleoperatorv1alpha1.AnsibleHost{}).
		Named("ansiblehost").
		Complete(r)
}
