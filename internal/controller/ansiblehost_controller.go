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

	ansibleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
	"github.com/LightJack05/ansible-operator/internal/ssh"
)

// AnsibleHostReconciler reconciles a AnsibleHost object
type AnsibleHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts/finalizers,verbs=update
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts/finalizers,verbs=update
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
	var ansibleHost ansibleoperatorv1alpha1.AnsibleHost
	if err := r.Get(ctx, req.NamespacedName, &ansibleHost); err != nil {
		// If the resource is not found, it might have been deleted after the reconcile request was queued.
		// In this case, we can ignore the error and return.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// NOTE: We use requeue instead of returning an error in order to make sure we don't hammer the SSH server and trigger rate limits or lockouts.

	// Ensure the host keys secret exists if we care about it
	if !ansibleHost.Spec.SSH.IgnoreHostKey {
		if err := r.ensureHostKeysSecretExists(ctx, &ansibleHost); err != nil {
			// If there was an error ensuring the host keys secret exists, we can log the error and requeue the request
			if err := r.setStatusNotReady(ctx, &ansibleHost, "HostKeysSecretError", fmt.Sprintf("Failed to ensure host keys secret exists: %v", err)); err != nil {
				lg.Error(fmt.Errorf("failed to update AnsibleHost status: %w", err), "AnsibleHost status update failed.")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}

			lg.Error(fmt.Errorf("failed to ensure host keys secret exists: %w", err), "Host keys secret error.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Set the host to ready once the result succeeds
	if err := r.setStatusReady(ctx, &ansibleHost); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update AnsibleHost status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AnsibleHostReconciler) setStatusReady(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) error {
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

func (r *AnsibleHostReconciler) setStatusNotReady(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost, reason, message string) error {
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

func (r *AnsibleHostReconciler) ensureHostKeysSecretExists(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) error {
	// Check if the host keys secret already exists
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ansibleHost.Namespace, Name: ansibleHost.Spec.SSH.SSHHostKeySecretRef.Name}, secret)
	if errors.IsNotFound(err) {
		// Secret does not exist, create it
		if err := r.createHostKeysSecret(ctx, ansibleHost); err != nil {
			return fmt.Errorf("failed to create host keys secret: %w", err)
		}
		return nil
	}

	if err != nil {
		// some other error, retry next reconcile
		return fmt.Errorf("failed to get host keys secret: %w", err)
	}

	// secret exists, nothing to do here
	return nil
}

func (r *AnsibleHostReconciler) createHostKeysSecret(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) error {
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

func getHostKeys(ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) (string, error) {
	keys, err := ssh.ScanHost(ansibleHost.Spec.Connection.Host, int(ansibleHost.Spec.Connection.Port))
	if err != nil {
		return "", fmt.Errorf("failed to scan host keys: %w", err)
	}

	keyString, err := ssh.HostKeysToSortedString(keys)
	if err != nil {
		return "", fmt.Errorf("failed to convert host keys to string: %w", err)
	}

	return keyString, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ansibleoperatorv1alpha1.AnsibleHost{}).
		Owns(&corev1.Secret{}).
		Named("ansiblehost").
		Complete(r)
}
