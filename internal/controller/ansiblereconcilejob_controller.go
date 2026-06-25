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
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ansibleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AnsibleReconcileJobReconciler reconciles a AnsibleReconcileJob object
type AnsibleReconcileJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts,verbs=get;list;watch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblegroups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AnsibleReconcileJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AnsibleReconcileJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := logf.FromContext(ctx)
	lg.Info("Reconciling AnsibleReconcileJob", "namespace", req.Namespace, "name", req.Name)

	// Get the AnsibleReconcileJob resource
	var reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob
	if err := r.Get(ctx, req.NamespacedName, &reconcileJob); err != nil {
		if errors.IsNotFound(err) {
			// The resource may have been deleted after the reconcile request was sent.
			// In this case, we can ignore the error and return.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Ensure...
	// ... the inventory configmap exists
	if err := r.ensureInventoryConfigmap(ctx, reconcileJob); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure inventory configmap: %w", err)
	}

	// ... the known hosts configmap exists
	if err := r.ensureKnownHostsSecretExists(ctx, reconcileJob); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure known hosts configmap: %w", err)
	}
	// ... the configuration configmap exists with all necessary keys
	// ... the cronjob exists and mounts all required files

	return ctrl.Result{}, nil
}

func (r *AnsibleReconcileJobReconciler) ensureKnownHostsSecretExists(ctx context.Context, reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	hosts := &ansibleoperatorv1alpha1.AnsibleHostList{}
	if err := r.List(ctx, hosts, client.InNamespace(reconcileJob.Namespace)); err != nil {
		return fmt.Errorf("failed to list AnsibleHosts in namespace %s: %w", reconcileJob.Namespace, err)
	}
	knownHosts, err := r.getKnownHostsForReconcileJob(ctx, hosts.Items)
	if err != nil {
		return fmt.Errorf("failed to get known hosts for reconcile job %s/%s: %w", reconcileJob.Namespace, reconcileJob.Name, err)
	}
	knownHostsString := strings.Join(knownHosts, "\n")
	err = r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+"-known-hosts", "known_hosts", knownHostsString, reconcileJob)
	if err != nil {
		return fmt.Errorf("failed to ensure known hosts configmap: %w", err)
	}
	return nil
}

func (r *AnsibleReconcileJobReconciler) getKnownHostsSecretForHost(ctx context.Context, host ansibleoperatorv1alpha1.AnsibleHost) (hostKeys string, err error) {
	secretName := host.Spec.SSH.SSHHostKeySecretRef.Name
	secretNamespace := host.Namespace

	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get secret %s/%s for host %s/%s: %w", secretNamespace, secretName, host.Namespace, host.Name, err)
	}
	secretValue, ok := secret.Data["host_keys"]
	if !ok {
		return "", fmt.Errorf("secret %s/%s for host %s/%s does not contain 'host_keys' key", secretNamespace, secretName, host.Namespace, host.Name)
	}
	return string(secretValue), nil
}

func (r *AnsibleReconcileJobReconciler) getKnownHostsForReconcileJob(ctx context.Context, hosts []ansibleoperatorv1alpha1.AnsibleHost) ([]string, error) {
	secrets := make([]string, 0, len(hosts))
	for _, host := range hosts {
		secret, err := r.getKnownHostsSecretForHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to get known hosts secret for host %s/%s: %w", host.Namespace, host.Name, err)
		}
		if secret == "" {
			logf.FromContext(ctx).Info(fmt.Sprintf("WARNING: skipping host %s/%s in known host key file since it does not have a know hosts secret", host.Namespace, host.Name))
			continue
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

func (r *AnsibleReconcileJobReconciler) ensureInventoryConfigmap(ctx context.Context, reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	hostMap, err := r.getAnsibleHostsInNamespace(ctx, reconcileJob.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get AnsibleHosts: %w", err)
	}

	groupMap, err := r.getAnsibleGroupsInNamespace(ctx, reconcileJob.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get AnsibleGroups: %w", err)
	}

	inventoryString, err := constructInventoryContent(hostMap, groupMap)
	if err != nil {
		return fmt.Errorf("failed to construct inventory content: %w", err)
	}
	err = r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+"-inventory", "inventory.yaml", inventoryString, reconcileJob)
	if err != nil {
		return fmt.Errorf("failed to ensure inventory configmap: %w", err)
	}

	return nil
}

func (r *AnsibleReconcileJobReconciler) ensureConfigmapWithKeyValue(ctx context.Context, name, key, value string, ansibleReconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	cm := corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: ansibleReconcileJob.Namespace}, &cm)
	if err == nil {
		// ConfigMap exists, check if the key exists and has the correct value
		if existingValue, ok := cm.Data[key]; ok {
			if existingValue == value {
				// Key exists and has the correct value, nothing to do
				return nil
			}
		}
		// Check if we own the configmap, if not, return an error
		if !isConfigmapOwnedByAnsibleReconcileJob(&cm, &ansibleReconcileJob) {
			return fmt.Errorf("configmap %s exists but is not owned by AnsibleReconcileJob", name)
		}

		// Update the key with the new value
		cm.Data[key] = value
		if err := r.Update(ctx, &cm); err != nil {
			return fmt.Errorf("failed to update configmap %s: %w", name, err)
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get configmap %s: %w", name, err)
	}

	// ConfigMap does not exist, create it
	cm.ObjectMeta = metav1.ObjectMeta{
		Name:      name,
		Namespace: ansibleReconcileJob.Namespace,
	}
	cm.Data = make(map[string]string)
	cm.Data[key] = value
	if err := ctrl.SetControllerReference(&ansibleReconcileJob, &cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference for configmap %s: %w", name, err)
	}
	if err := r.Create(ctx, &cm); err != nil {
		return fmt.Errorf("failed to create configmap %s: %w", name, err)
	}
	return nil
}

func isConfigmapOwnedByAnsibleReconcileJob(cm *corev1.ConfigMap, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob) bool {
	for _, ownerRef := range cm.OwnerReferences {
		if ownerRef.Kind == "AnsibleReconcileJob" && ownerRef.Name == reconcileJob.Name {
			return true
		}
	}
	return false
}

type InventoryContentGroup struct {
	Name      string
	Subgroups []string
	Hosts     []string
}

type InventoryContentHost struct {
	InventoryName string
	Hostname      string
	Port          uint16
	Username      string
	Become        bool
}
type InventoryContent struct {
	Groups []InventoryContentGroup
	Hosts  []InventoryContentHost
}

func constructInventoryContent(hostMap map[string]ansibleoperatorv1alpha1.AnsibleHost, groupMap map[string]ansibleoperatorv1alpha1.AnsibleGroup) (string, error) {
	inventoryContent := InventoryContent{
		Groups: []InventoryContentGroup{},
		Hosts:  []InventoryContentHost{},
	}

	for _, host := range hostMap {
		inventoryContent.Hosts = append(inventoryContent.Hosts, mapAnsibleHost(host))
	}

	for _, group := range groupMap {
		inventoryGroupEntry := InventoryContentGroup{
			Name: group.Spec.AnsibleName,
		}
		for _, groupRef := range group.Spec.Groups {
			key, err := mapMemberGroup(groupRef.Name, groupMap)
			if err != nil {
				return "", fmt.Errorf("failed to construct inventory at group %s for subgroup %s: %w", group.Name, groupRef.Name, err)
			}
			inventoryGroupEntry.Subgroups = append(inventoryGroupEntry.Subgroups, key)
		}

		for _, hostRef := range group.Spec.Hosts {
			key, err := mapMemberHost(hostRef.Name, hostMap)
			if err != nil {
				return "", fmt.Errorf("failed to construct inventory at group %s for host %s: %w", group.Name, hostRef.Name, err)
			}
			inventoryGroupEntry.Hosts = append(inventoryGroupEntry.Hosts, key)
		}
		inventoryContent.Groups = append(inventoryContent.Groups, inventoryGroupEntry)
	}

	inventoryString, err := renderInventoryTemplate(inventoryContent)
	if err != nil {
		return "", fmt.Errorf("failed to construct inventory: %w", err)
	}
	return inventoryString, nil
}

func renderInventoryTemplate(inventory InventoryContent) (string, error) {
	inventoryTemplate := `all:
  hosts:
{{- range .Hosts }}
    {{ .InventoryName }}:
    {{- if .Hostname }}
      ansible_host: {{ .Hostname }}
    {{- end }}
    {{- if .Port }}
      ansible_port: {{ .Port }}
    {{- end }}
    {{- if .Username }}
      ansible_user: {{ .Username }}
    {{- end }}
    {{- if .Become }}
      ansible_become: true
    {{- end }}
{{- end }}
{{ range .Groups }}
{{- .Name }}:
{{- if .Subgroups }}
  children:
  {{- range .Subgroups }}
    {{ . }}:
  {{- end }}
{{- end }}
{{- if .Hosts }}
  hosts:
  {{- range .Hosts }}
    {{ . }}:
  {{- end }}
{{- end }}
{{ end -}}
`
	inventoryStringBuilder := strings.Builder{}
	tmpl := template.Must(template.New("inv").Parse(inventoryTemplate))
	if err := tmpl.Execute(&inventoryStringBuilder, inventory); err != nil {
		return "", fmt.Errorf("unable to template inventory: %w", err)
	}

	return inventoryStringBuilder.String(), nil
}

func mapMemberGroup(key string, groupMap map[string]ansibleoperatorv1alpha1.AnsibleGroup) (string, error) {
	group, ok := groupMap[key]
	if !ok {
		return "", fmt.Errorf("unresolved child group reference %s", key)
	}
	return group.Spec.AnsibleName, nil
}

func mapMemberHost(key string, hostMap map[string]ansibleoperatorv1alpha1.AnsibleHost) (string, error) {
	host, ok := hostMap[key]
	if !ok {
		return "", fmt.Errorf("unresolved host reference %s", key)
	}
	return host.Spec.AnsibleName, nil
}

func mapAnsibleHost(host ansibleoperatorv1alpha1.AnsibleHost) InventoryContentHost {
	return InventoryContentHost{
		InventoryName: host.Spec.AnsibleName,
		Hostname:      host.Spec.Connection.Host,
		Port:          host.Spec.Connection.Port,
		Username:      host.Spec.Connection.User,
		Become:        host.Spec.Privilege.Become,
	}
}

func (r *AnsibleReconcileJobReconciler) getAnsibleGroupsInNamespace(ctx context.Context, namespace string) (groupMap map[string]ansibleoperatorv1alpha1.AnsibleGroup, err error) {
	groupMap = make(map[string]ansibleoperatorv1alpha1.AnsibleGroup)
	groups := &ansibleoperatorv1alpha1.AnsibleGroupList{}
	if err := r.List(ctx, groups, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AnsibleGroups in namespace %s: %w", namespace, err)
	}
	for _, group := range groups.Items {
		groupMap[group.Name] = group
	}
	return groupMap, nil
}

func (r *AnsibleReconcileJobReconciler) getAnsibleHostsInNamespace(ctx context.Context, namespace string) (hostMap map[string]ansibleoperatorv1alpha1.AnsibleHost, err error) {
	hostMap = make(map[string]ansibleoperatorv1alpha1.AnsibleHost)
	hosts := &ansibleoperatorv1alpha1.AnsibleHostList{}
	if err := r.List(ctx, hosts, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AnsibleHosts in namespace %s: %w", namespace, err)
	}
	for _, host := range hosts.Items {
		hostMap[host.Name] = host
	}
	return hostMap, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleReconcileJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ansibleoperatorv1alpha1.AnsibleReconcileJob{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&ansibleoperatorv1alpha1.AnsibleHost{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfChangedHostInNamespace)).
		Watches(&ansibleoperatorv1alpha1.AnsibleGroup{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfChangedGroupInNamespace)).
		Named("ansiblereconcilejob").
		Complete(r)
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfChangedHostInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	requests, err := r.newMethod(ctx, obj.GetNamespace())
	if err != nil {
		logf.Log.Error(err, "failed to enqueue AnsibleReconcileJobs for AnsibleHost change")
		return []reconcile.Request{}
	}
	return requests
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfChangedGroupInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	requests, err := r.newMethod(ctx, obj.GetNamespace())
	if err != nil {
		logf.Log.Error(err, "failed to enqueue AnsibleReconcileJobs for AnsibleGroup change")
		return []reconcile.Request{}
	}
	return requests
}

func (r *AnsibleReconcileJobReconciler) newMethod(ctx context.Context, namespace string) ([]reconcile.Request, error) {
	requests := []reconcile.Request{}
	// List all AnsibleReconcileJobs in the same namespace
	reconcileJobs := &ansibleoperatorv1alpha1.AnsibleReconcileJobList{}
	if err := r.List(ctx, reconcileJobs, client.InNamespace(namespace)); err != nil {
		logf.Log.Error(err, "failed to list AnsibleReconcileJobs")
		return requests, fmt.Errorf("failed to list AnsibleReconcileJobs in namespace %s: %w", namespace, err)
	}

	// Enqueue a reconcile request for each AnsibleReconcileJob
	for _, job := range reconcileJobs.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      job.Name,
				Namespace: job.Namespace,
			},
		})
	}
	return requests, nil
}
