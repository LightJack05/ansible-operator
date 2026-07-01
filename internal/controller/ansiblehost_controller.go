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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ansibleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
	"github.com/LightJack05/ansible-operator/internal/ssh"
	cryptoSSH "golang.org/x/crypto/ssh"
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
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
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

	// Ensure the private key secret exists
	privateKeySecretExists, err := r.hostPrivateKeySecretExists(ctx, &ansibleHost)
	if err != nil {
		// If there was an error checking for the private key secret, we can log the error and requeue the request
		if err := r.setStatusNotReady(ctx, &ansibleHost, "PrivateKeySecretError", fmt.Sprintf("Failed to check if private key secret exists: %v", err)); err != nil {
			lg.Error(err, "AnsibleHost status update failed.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		lg.Error(err, "Private key secret error.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if !privateKeySecretExists {
		// If the private key secret does not exist, we can log the error and requeue the request
		if err := r.setStatusNotReady(ctx, &ansibleHost, "PrivateKeySecretMissing", "The private key secret does not exist"); err != nil {
			lg.Error(err, "AnsibleHost status update failed.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		lg.Info("Private key secret does not exist, requeuing.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Ensure the host keys secret exists if we care about it
	if !ansibleHost.Spec.SSH.IgnoreHostKey {
		if err := r.ensureHostKeysSecretExists(ctx, &ansibleHost); err != nil {
			// If there was an error ensuring the host keys secret exists, we can log the error and requeue the request
			if err := r.setStatusNotReady(ctx, &ansibleHost, "HostKeysSecretError", fmt.Sprintf("Failed to ensure host keys secret exists: %v", err)); err != nil {
				lg.Error(err, "AnsibleHost status update failed.")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}

			lg.Error(err, "Host keys secret error.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Ensure the host vars configmap exists
	if err := r.ensureVarsConfigMap(ctx, &ansibleHost, ansibleHost.Spec.HostVars); err != nil {
		// If there was an error ensuring the vars configmap exists, we can log the error and requeue the request
		if err := r.setStatusNotReady(ctx, &ansibleHost, "VarsConfigMapError", fmt.Sprintf("Failed to ensure vars configmap exists: %v", err)); err != nil {
			lg.Error(err, "AnsibleHost status update failed.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		lg.Error(err, "Vars configmap error.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Set the host to ready once the result succeeds
	if err := r.setStatusReady(ctx, &ansibleHost); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update AnsibleHost status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AnsibleHostReconciler) ensureVarsConfigMap(ctx context.Context, host *ansibleoperatorv1alpha1.AnsibleHost, vars string) error {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: host.Namespace, Name: host.Name + varsConfigMapSuffix}, cm); err != nil {
		if errors.IsNotFound(err) {
			// Create the ConfigMap if it doesn't exist
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      host.Name + varsConfigMapSuffix,
					Namespace: host.Namespace,
				},
				Data: map[string]string{
					hostVarsConfigMapKey: vars,
				},
			}
			if err := ctrl.SetControllerReference(host, cm, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner reference for ConfigMap: %w", err)
			}
			if err := r.Create(ctx, cm); err != nil {
				return fmt.Errorf("failed to create ConfigMap for AnsibleHost vars: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get ConfigMap for AnsibleHost vars: %w", err)
		}
	}

	// Check the owner reference and update the vars if it is owned by the AnsibleHost
	if metav1.IsControlledBy(cm, host) {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		if cm.Data[hostVarsConfigMapKey] != vars {
			cm.Data[hostVarsConfigMapKey] = vars
			if err := r.Update(ctx, cm); err != nil {
				return fmt.Errorf("failed to update ConfigMap for AnsibleHost vars: %w", err)
			}
		}
	} else {
		return fmt.Errorf("ConfigMap %s is not owned by AnsibleHost %s", cm.Name, host.Name)
	}

	return nil
}
func (r *AnsibleHostReconciler) hostPrivateKeySecretExists(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) (bool, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ansibleHost.Namespace, Name: ansibleHost.Spec.SSH.SSHKeySecretRef.Name}, secret)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get private key secret: %w", err)
	}

	keyData, ok := secret.Data[sshKeySecretKey]
	if !ok || len(keyData) == 0 {
		return false, fmt.Errorf("private key secret is missing 'ssh_key' data or it is empty")
	}

	if _, err := cryptoSSH.ParsePrivateKey(keyData); err != nil {
		return false, fmt.Errorf("failed to parse private key: %w", err)
	}

	return true, nil
}

func (r *AnsibleHostReconciler) setStatusReady(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) error {
	changed := meta.SetStatusCondition(&ansibleHost.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "HostReady",
		Message: "The AnsibleHost is ready to be used",
	})
	if changed {
		err := r.Status().Update(ctx, ansibleHost)
		if err != nil {
			return fmt.Errorf("failed to update AnsibleHost status: %w", err)
		}
	}
	return nil
}

func (r *AnsibleHostReconciler) setStatusNotReady(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost, reason, message string) error {
	changed := meta.SetStatusCondition(&ansibleHost.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	if changed {
		err := r.Status().Update(ctx, ansibleHost)
		if err != nil {
			return fmt.Errorf("failed to update AnsibleHost status: %w", err)
		}
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
	hostKeys, err := getHostKeys(ctx, ansibleHost)
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

func getHostKeys(ctx context.Context, ansibleHost *ansibleoperatorv1alpha1.AnsibleHost) (string, error) {
	keys, err := ssh.ScanHost(ctx, ansibleHost.Spec.Connection.Host, int(ansibleHost.Spec.Connection.Port))
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
		Owns(&corev1.ConfigMap{}).
		Named("ansiblehost").
		// Run at most 100 concurrent reconciles.
		// The high number here is irrelevant, since the threads aren't CPU bound, but just blocked on network IO for the most time.
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		Complete(r)
}
