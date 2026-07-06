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

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ansibleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// AnsibleReconcileJobReconciler reconciles a AnsibleReconcileJob object
type AnsibleReconcileJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const operatorName = "ansible-operator"

const (
	jobLabelOwnerReconcileJob = "ansible-operator.lightjack.de/owner-reconcile-job"
)

// Runtime config map keys and name
const (
	jobConfigNameSuffix                 = `-job-config`
	varsConfigMapSuffix                 = `-vars`
	knownHostsConfigMapNameSuffix       = "-known-hosts"
	inventoryConfigMapNameSuffix        = "-inventory"
	runtimeConfigGitRefKey              = `OPERATOR_GIT_REF`
	runtimeConfigGitRepoUrlKey          = `OPERATOR_GIT_REPO_URL`
	runtimeConfigGitPlaybookPathKey     = `OPERATOR_GIT_PLAYBOOK_PATH`
	runtimeConfigGitRequirementsPathKey = `OPERATOR_GIT_REQUIREMENTS_PATH`
	runtimeConfigPlaybookYAMLKey        = `playbook.yml`
	runtimeConfigRequirementsYAMLKey    = `requirements.yml`
	inventoryConfigMapKey               = "inventory.yaml"
	knownHostsConfigMapKey              = "known_hosts"
	sshKeySecretKey                     = "ssh_key"
	groupVarsConfigMapKey               = "vars"
	hostVarsConfigMapKey                = "vars"
)

const (
	runtimeConfigGitRefEnvVar              = runtimeConfigGitRefKey
	runtimeConfigGitRepoUrlEnvVar          = runtimeConfigGitRepoUrlKey
	runtimeConfigGitPlaybookPathEnvVar     = runtimeConfigGitPlaybookPathKey
	runtimeConfigGitRequirementsPathEnvVar = runtimeConfigGitRequirementsPathKey
)

const (
	ociImageJobInitContainer    = "ghcr.io/lightjack05/ansible-operator-runner-init:latest"
	ociImageJobRuntimeContainer = "ghcr.io/lightjack05/ansible-operator-runner-runner:latest"
)

var (
	emptyDirSizeLimit = resource.MustParse("5Gi")
)

const (
	mountSuffix       = `-mount`
	hostMountSuffix   = `-host` + mountSuffix
	groupMountSuffix  = `-group` + mountSuffix
	sshKeyMountSuffix = `-key` + mountSuffix
)

const (
	playbooksVolumeName     = "playbooks"
	dependenciesVolumeName  = "deps"
	inventoryVolumeName     = "inventory"
	knownHostsVolumeName    = "knownhosts"
	runtimeConfigVolumeName = "runtimeconfig"
)

const (
	inventoryDir             = "/inventory/"
	inventoryGroupVarsDir    = inventoryDir + "group_vars/"
	inventoryHostVarsDir     = inventoryDir + "host_vars/"
	playbooksEmptyDirPath    = "/playbook"
	dependenciesEmptyDirPath = "/deps"
	runtimeConfigMountPath   = "/config"
)

const (
	inventoryHostsFileName = "hosts.yaml"
	inventoryHostsFilePath = inventoryDir + inventoryHostsFileName
	knownHostsFileName     = "known_hosts"
	knownHostsFilePath     = "/ssh/known_hosts"
	playbookFileName       = "playbook.yml"
	playbookFilePath       = playbooksEmptyDirPath + playbookFileName
	requirementsFileName   = "requirements.yml"
	requirementsFilePath   = playbooksEmptyDirPath + playbookFileName
	sshKeysDirPath         = "/ssh/keys/"
)

// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblereconcilejobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblehosts,verbs=get;list;watch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblegroups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AnsibleReconcileJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AnsibleReconcileJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
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
		return ctrl.Result{}, fmt.Errorf("failed to get AnsibleReconcileJob: %w", err)
	}

	// set all status conditions to unknown if they are not set already
	if err = r.defaultAllStatusesToUnknown(ctx, &reconcileJob); err != nil {
		err = fmt.Errorf("unable to set statuses to initial values: %w", err)
		goto err
	}

	// Ensure...
	// ... the inventory configmap exists
	err = r.ensureInventoryConfigmap(ctx, reconcileJob)
	if err != nil {
		err = fmt.Errorf("failed to ensure inventory configmap: %w", err)
		goto err
	}
	// ... the known hosts configmap exists
	err = r.ensureKnownHostsSecretExists(ctx, reconcileJob)
	if err != nil {
		err = fmt.Errorf("failed to ensure known hosts configmap: %w", err)
		goto err
	}
	// ... the configuration configmap exists with all necessary keys
	err = r.ensureRuntimeConfigMap(ctx, reconcileJob)
	if err != nil {
		err = fmt.Errorf("failed to ensure runtime configmap: %w", err)
		goto err
	}
	// ... the cronjob exists and mounts all required files
	err = r.ensureCronjobWithMounts(ctx, reconcileJob)
	if err != nil {
		err = fmt.Errorf("failed to ensure cronjob exists: %w", err)
		goto err
	}

	if err = r.updateStatusConditions(ctx, &reconcileJob); err != nil {
		err = fmt.Errorf("failed to update status conditions: %w", err)
		goto err
	}

	if err = r.setCondition(ctx, &reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionReady, metav1.ConditionTrue, "ReconcileSuccess", "Resource reconciled successfully"); err != nil {
		err = fmt.Errorf("failed to update condition to ready on reconcileJob: %w", err)
		goto err
	}

	return ctrl.Result{}, nil
err:
	r.handleReconcileError(ctx, reconcileJob, err)
	return ctrl.Result{}, fmt.Errorf("error encountered during reconcile: %w", err)
}

func (r *AnsibleReconcileJobReconciler) updateStatusConditions(ctx context.Context, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	// Get any jobs in the current namespace to check if any are running
	jobs := batchv1.JobList{}
	if err := r.List(ctx, &jobs, client.InNamespace(reconcileJob.Namespace), client.MatchingLabels{jobLabelOwnerReconcileJob: reconcileJob.Name}); err != nil {
		return fmt.Errorf("unable to fetch matching jobs: %w", err)
	}

	var newestJob *batchv1.Job
	for _, job := range jobs.Items {
		if job.Status.StartTime == nil {
			continue
		}
		if newestJob == nil || job.Status.StartTime.After(newestJob.Status.StartTime.Time) {
			newestJob = &job
		}
	}

	// if no job is found, set the conditions to unknown and return nil
	if newestJob == nil {
		for _, condition := range []string{ansibleoperatorv1alpha1.AnsibleReconcileJobConditionSuccessful, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionProgressing} {
			if err := r.setCondition(ctx, reconcileJob, condition, metav1.ConditionUnknown, "NoJobsFound", "There are currently no jobs present matching this reconcileJob"); err != nil {
				return fmt.Errorf("failed to set status to unknown on condition %s: %w", condition, err)
			}
		}
		return nil
	}

	if err := r.updateProgressingStatus(ctx, newestJob, reconcileJob); err != nil {
		return fmt.Errorf("failed to update progressing status: %w", err)
	}
	if err := r.updateSuccessfulStatus(ctx, newestJob, reconcileJob); err != nil {
		return fmt.Errorf("failed to update successful status: %w", err)
	}

	return nil
}

func (r *AnsibleReconcileJobReconciler) updateSuccessfulStatus(ctx context.Context, newestJob *batchv1.Job, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	if newestJob.Status.Succeeded > 0 {
		if err := r.setCondition(ctx, reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionSuccessful, metav1.ConditionTrue, "JobSucceeded", "The last job for this reconcileJob completed successfully"); err != nil {
			return fmt.Errorf("failed to set Successful status: %w", err)
		}
		return nil
	} else if newestJob.Status.Failed > 0 {
		if err := r.setCondition(ctx, reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionSuccessful, metav1.ConditionFalse, "JobFailed", "The last job for this reconcileJob failed, check logs for details"); err != nil {
			return fmt.Errorf("failed to set Successful status: %w", err)
		}
		return nil
	}
	// The job has neither successful nor failed runs, it is probably still initializing. Set the status to unknown.
	if err := r.setCondition(ctx, reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionSuccessful, metav1.ConditionUnknown, "JobStatusUnknown", "The last job for this reconcileJob has no failed or succeeded runs yet."); err != nil {
		return fmt.Errorf("failed to set Successful status: %w", err)
	}
	return nil
}

func (r *AnsibleReconcileJobReconciler) updateProgressingStatus(ctx context.Context, newestJob *batchv1.Job, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	if newestJob.Status.Active > 0 {
		// Newest job is running
		if err := r.setCondition(ctx, reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionProgressing, metav1.ConditionTrue, "JobRunning", "A job belonging to this reconcileJob is currently running"); err != nil {
			return fmt.Errorf("failed to set progressing condition on ansible recoconcile job: %w", err)
		}
	} else {
		// Newest job has failed or is complete
		if err := r.setCondition(ctx, reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionProgressing, metav1.ConditionFalse, "JobNotRunning", "There is no job currently running for this reconcileJob"); err != nil {
			return fmt.Errorf("failed to set progressing condition on ansible recoconcile job: %w", err)
		}
	}
	return nil
}

func (r *AnsibleReconcileJobReconciler) defaultAllStatusesToUnknown(ctx context.Context, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	// Set all condition default to unknown if they do not have a status yet
	for _, condition := range []string{
		ansibleoperatorv1alpha1.AnsibleReconcileJobConditionReady,
		ansibleoperatorv1alpha1.AnsibleReconcileJobConditionProgressing,
		ansibleoperatorv1alpha1.AnsibleReconcileJobConditionSuccessful,
	} {
		if err := r.defaultStatusToUnknown(ctx, reconcileJob, condition); err != nil {
			return fmt.Errorf("failed to default condition %s of reconcile job %s to unknown: %w", condition, reconcileJob.Name, err)
		}
	}

	return nil
}

func (r *AnsibleReconcileJobReconciler) defaultStatusToUnknown(ctx context.Context, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob, status string) error {
	if meta.FindStatusCondition(reconcileJob.Status.Conditions, status) == nil {
		if err := r.setCondition(ctx, reconcileJob, status, metav1.ConditionUnknown, "ReconcileStarted", fmt.Sprintf("AnsibleGroup is initializing: %s condition not set", status)); err != nil {
			return fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	}
	return nil
}

func (r *AnsibleReconcileJobReconciler) setCondition(ctx context.Context, reconcileJob *ansibleoperatorv1alpha1.AnsibleReconcileJob, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) error {
	changed := meta.SetStatusCondition(&reconcileJob.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})

	if changed {
		if err := r.Status().Update(ctx, reconcileJob); err != nil {
			return fmt.Errorf("unable to update AnsibleGroup status: %v", err)
		}
	}

	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}

func (r *AnsibleReconcileJobReconciler) ensureCronjobWithMounts(ctx context.Context, reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	hosts := &ansibleoperatorv1alpha1.AnsibleHostList{}
	groups := &ansibleoperatorv1alpha1.AnsibleGroupList{}

	if err := r.List(ctx, hosts, client.InNamespace(reconcileJob.Namespace)); err != nil {
		return fmt.Errorf("unable to list hosts in namespace %s: %w", reconcileJob.Namespace, err)
	}
	if err := r.List(ctx, groups, client.InNamespace(reconcileJob.Namespace)); err != nil {
		return fmt.Errorf("unable to list groups in namespace %s: %w", reconcileJob.Namespace, err)
	}

	hostSSHKeyPairs := make([]ownerOwnedWithAnsibleNamePair, 0, len(hosts.Items))
	hostVarPairs := make([]ownerOwnedWithAnsibleNamePair, 0, len(hosts.Items))
	groupVarPairs := make([]ownerOwnedWithAnsibleNamePair, 0, len(groups.Items))

	for _, host := range hosts.Items {
		hostSSHKeyPairs = append(hostSSHKeyPairs, ownerOwnedWithAnsibleNamePair{
			OwnerAnsibleName: host.Spec.AnsibleName,
			Owner:            host.Name,
			Owned:            host.Spec.SSH.SSHKeySecretRef.Name,
		})

		hostVarPairs = append(hostVarPairs, ownerOwnedWithAnsibleNamePair{
			OwnerAnsibleName: host.Spec.AnsibleName,
			Owner:            host.Name,
			Owned:            host.Name + varsConfigMapSuffix,
		})
	}

	for _, group := range groups.Items {
		groupVarPairs = append(groupVarPairs, ownerOwnedWithAnsibleNamePair{
			OwnerAnsibleName: group.Spec.AnsibleName,
			Owner:            group.Name,
			Owned:            group.Name + varsConfigMapSuffix,
		})
	}

	cj := constructCronjobWithMounts(
		reconcileJob.Name,
		reconcileJob.Namespace,
		reconcileJob.Spec.Schedule,
		reconcileJob.Name,
		reconcileJob.Name+jobConfigNameSuffix,
		reconcileJob.Name+knownHostsConfigMapNameSuffix,
		reconcileJob.Name+inventoryConfigMapNameSuffix,
		hostSSHKeyPairs,
		hostVarPairs,
		groupVarPairs)

	if err := ctrl.SetControllerReference(&reconcileJob, cj, r.Scheme); err != nil {
		return fmt.Errorf("unable to set owner reference on cronjob: %w", err)
	}

	// NOTE: Stupid dance because of deprecated API for apply. Clean this up once there's time
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cj)
	if err != nil {
		return fmt.Errorf("failed to unstructure object: %w", err)
	}
	u := &unstructured.Unstructured{Object: m}
	u.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("CronJob"))
	if err := r.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner(operatorName), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to server-side apply cronjob for reconcileJob %s: %w", reconcileJob.Name, err)
	}

	return nil
}

type ownerOwnedWithAnsibleNamePair struct {
	OwnerAnsibleName string
	Owner            string
	Owned            string
}

func constructCronjobWithMounts(name, namespace, schedule, reconcileJobName string, runtimeConfigMapName, knownHostsConfigMapName, inventoryConfigMapName string, keySecrets, hostVarsEntries, groupVarsEntries []ownerOwnedWithAnsibleNamePair) *batchv1.CronJob {
	// TODO: Inject env vars for non-inline playbooks
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          schedule,
			Suspend:           boolPtr(false),
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						jobLabelOwnerReconcileJob: reconcileJobName,
					},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							InitContainers: []corev1.Container{
								{
									Name:  "init",
									Image: ociImageJobInitContainer,
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      playbooksVolumeName,
											ReadOnly:  false,
											MountPath: playbooksEmptyDirPath,
										},
										{
											Name:      dependenciesVolumeName,
											ReadOnly:  false,
											MountPath: dependenciesEmptyDirPath,
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  "runner",
									Image: ociImageJobRuntimeContainer,
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      playbooksVolumeName,
											ReadOnly:  true,
											MountPath: playbooksEmptyDirPath,
										},
										{
											Name:      dependenciesVolumeName,
											ReadOnly:  true,
											MountPath: dependenciesEmptyDirPath,
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: playbooksVolumeName,
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{
											SizeLimit: &emptyDirSizeLimit,
										},
									},
								},
								{
									Name: dependenciesVolumeName,
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{
											SizeLimit: &emptyDirSizeLimit,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	makeEnvVarEntryFromConfigMap := func(variable, configmap, key string) corev1.EnvVar {
		return corev1.EnvVar{
			Name: variable,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configmap,
					},
					Key:      key,
					Optional: boolPtr(true),
				},
			},
		}
	}

	// Env vars
	cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers[0].Env = []corev1.EnvVar{
		makeEnvVarEntryFromConfigMap(runtimeConfigGitRepoUrlEnvVar, runtimeConfigMapName, runtimeConfigGitRepoUrlKey),
		makeEnvVarEntryFromConfigMap(runtimeConfigGitRefEnvVar, runtimeConfigMapName, runtimeConfigGitRefKey),
		makeEnvVarEntryFromConfigMap(runtimeConfigGitPlaybookPathEnvVar, runtimeConfigMapName, runtimeConfigGitPlaybookPathKey),
		makeEnvVarEntryFromConfigMap(runtimeConfigGitRequirementsPathEnvVar, runtimeConfigMapName, runtimeConfigGitRequirementsPathKey),
	}

	// runtime config (inline)
	runtimeConfigVolume := corev1.Volume{
		Name: runtimeConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				Optional: boolPtr(true),
				LocalObjectReference: corev1.LocalObjectReference{
					Name: runtimeConfigMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  runtimeConfigPlaybookYAMLKey,
						Path: playbookFileName,
					},
					{
						Key:  runtimeConfigRequirementsYAMLKey,
						Path: requirementsFileName,
					},
				},
			},
		},
	}

	runtimeConfigMount := corev1.VolumeMount{
		Name:      runtimeConfigVolumeName,
		ReadOnly:  true,
		MountPath: runtimeConfigMountPath,
	}

	cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, runtimeConfigVolume)
	cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, runtimeConfigMount)
	cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers[0].VolumeMounts, runtimeConfigMount)

	// inventory hosts.yaml
	cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, *buildVolumeForConfigMap(inventoryVolumeName, inventoryConfigMapName, inventoryConfigMapKey, inventoryHostsFileName))

	cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, *buildReadOnlyVolumeMount(inventoryVolumeName, inventoryHostsFilePath, inventoryHostsFileName))

	// ssh known hosts
	cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, *buildVolumeForConfigMap(knownHostsVolumeName, knownHostsConfigMapName, knownHostsConfigMapKey, knownHostsFileName))
	cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, *buildReadOnlyVolumeMount(knownHostsVolumeName, knownHostsFilePath, knownHostsFileName))

	// NOTE: These object names should be unique because the K8s API enforces unique names for objects within a namespace
	// ssh keys
	for _, sshKeyEntry := range keySecrets {
		volumeName := sshKeyEntry.Owner + sshKeyMountSuffix
		volume := buildVolumeForSecret(volumeName, sshKeyEntry.Owned, sshKeySecretKey, sshKeyEntry.Owner)
		mount := buildReadOnlyVolumeMount(volumeName, sshKeysDirPath+sshKeyEntry.Owner, sshKeyEntry.Owner)
		cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, *volume)
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, *mount)
	}

	// Group vars
	for _, groupVars := range groupVarsEntries {
		volumeName := groupVars.Owner + groupMountSuffix
		volume := buildVolumeForConfigMap(volumeName, groupVars.Owned, groupVarsConfigMapKey, groupVars.Owner)
		mount := buildReadOnlyVolumeMount(volumeName, inventoryGroupVarsDir+groupVars.OwnerAnsibleName, groupVars.Owner)
		cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, *volume)
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, *mount)
	}

	// Host vars
	for _, hostVars := range hostVarsEntries {
		volumeName := hostVars.Owner + hostMountSuffix
		volume := buildVolumeForConfigMap(volumeName, hostVars.Owned, hostVarsConfigMapKey, hostVars.Owner)
		mount := buildReadOnlyVolumeMount(volumeName, inventoryHostVarsDir+hostVars.OwnerAnsibleName, hostVars.Owner)
		cj.Spec.JobTemplate.Spec.Template.Spec.Volumes = append(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes, *volume)
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = append(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts, *mount)
	}

	return cj
}

func buildReadOnlyVolumeMount(name, mountPath, subPath string) *corev1.VolumeMount {
	return &corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		SubPath:   subPath,
		ReadOnly:  true,
	}
}

func buildVolumeForSecret(name, secret, key, path string) *corev1.Volume {
	return &corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secret,
				Items: []corev1.KeyToPath{
					{
						Key:  key,
						Path: path,
						Mode: int32Ptr(0o600),
					},
				},
			},
		},
	}
}

func buildVolumeForConfigMap(name, configmap, key, path string) *corev1.Volume {
	return &corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configmap,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  key,
						Path: path,
					},
				},
			},
		},
	}
}

func (r *AnsibleReconcileJobReconciler) handleReconcileError(ctx context.Context, reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob, cause error) {
	// NOTE: It would be pointless to return an error here, so we just try our best and log any errors, before requeuing the resource
	lg := logf.FromContext(ctx)
	lg.Info("Encountered an error during reconcile. Attempting to disable cronjob and set the ReconcileJob into error state...")
	cronjob := &batchv1.CronJob{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: reconcileJob.Namespace, Name: reconcileJob.Name}, cronjob); err != nil {
		lg.Error(err, "unable to fetch cronjob matching reconcileJob")
	}
	cronjob.Spec.Suspend = boolPtr(true)
	if err := r.Update(ctx, cronjob); err != nil {
		lg.Error(err, "unable to update cronjob to disable subsequent executions")
	}

	if err := r.setCondition(ctx, &reconcileJob, ansibleoperatorv1alpha1.AnsibleReconcileJobConditionReady, metav1.ConditionFalse, "ReconcileError", cause.Error()); err != nil {
		lg.Error(err, "unable to set Ready condition on reconcileJob")
	}
}

func (r *AnsibleReconcileJobReconciler) ensureRuntimeConfigMap(ctx context.Context, reconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	// Get the referenced playbook
	playbook := ansibleoperatorv1alpha1.AnsiblePlaybook{}
	err := r.Get(ctx, client.ObjectKey{Namespace: reconcileJob.Namespace, Name: reconcileJob.Spec.PlaybookRef.Name}, &playbook)
	if err != nil {
		return fmt.Errorf("unable to fetch matching playbook: %w", err)
	}

	// TODO: Ensure the keys are absent when they are not needed
	if playbook.Spec.Inline != nil {
		// inline playbook, mount the strings as files
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigPlaybookYAMLKey, playbook.Spec.Inline.Playbook, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigPlaybookYAMLKey, err)
		}
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigRequirementsYAMLKey, playbook.Spec.Inline.Requirements, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigRequirementsYAMLKey, err)
		}
	} else if playbook.Spec.Git != nil {
		// git sourced playbook, pass the source and paths to the init container
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRepoUrlKey, playbook.Spec.Git.Repo.URL, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRepoUrlKey, err)
		}
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRefKey, playbook.Spec.Git.Repo.Ref, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRefKey, err)
		}
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitPlaybookPathKey, playbook.Spec.Git.PlaybookPath, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitPlaybookPathKey, err)
		}
		if err := r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRequirementsPathKey, playbook.Spec.Git.RequirementsPath, reconcileJob); err != nil {
			return fmt.Errorf("error ensuring configmap %s for key %s: %w", reconcileJob.Name+jobConfigNameSuffix, runtimeConfigGitRequirementsPathKey, err)
		}
	} else {
		return fmt.Errorf("error creating runtime configmap: neither git nor inline are non-nil. (API Spec violation?!)")
	}
	return nil
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
	err = r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+knownHostsConfigMapNameSuffix, knownHostsConfigMapKey, knownHostsString, reconcileJob)
	if err != nil {
		return fmt.Errorf("failed to ensure known hosts configmap: %w", err)
	}
	return nil
}

func (r *AnsibleReconcileJobReconciler) getKnownHostsLinesForHost(ctx context.Context, host ansibleoperatorv1alpha1.AnsibleHost) (hostKeys string, err error) {
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
	// Prepend the hostname of the ansible host to each line in the ssh known hosts keys
	for line := range strings.SplitSeq(string(secretValue), "\n") {
		if line == "" {
			continue
		}
		hostKeys += fmt.Sprintf("%s %s\n", host.Spec.Connection.Host, line)
	}
	return hostKeys, nil
}

func (r *AnsibleReconcileJobReconciler) getKnownHostsForReconcileJob(ctx context.Context, hosts []ansibleoperatorv1alpha1.AnsibleHost) ([]string, error) {
	secrets := make([]string, 0, len(hosts))
	for _, host := range hosts {
		secret, err := r.getKnownHostsLinesForHost(ctx, host)
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
	err = r.ensureConfigmapWithKeyValue(ctx, reconcileJob.Name+inventoryConfigMapNameSuffix, inventoryConfigMapKey, inventoryString, reconcileJob)
	if err != nil {
		return fmt.Errorf("failed to ensure inventory configmap: %w", err)
	}

	return nil
}

func (r *AnsibleReconcileJobReconciler) ensureConfigmapWithKeyValue(ctx context.Context, name, key, value string, ansibleReconcileJob ansibleoperatorv1alpha1.AnsibleReconcileJob) error {
	cm := corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: ansibleReconcileJob.Namespace}, &cm)
	if err == nil {
		// ensure we don't deref nil
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
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
	KubernetesName string
	InventoryName  string
	Hostname       string
	Port           uint16
	Username       string
	Become         bool
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
      ansible_ssh_private_key_file: /ssh/keys/{{.KubernetesName}}
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
		KubernetesName: host.Name,
		InventoryName:  host.Spec.AnsibleName,
		Hostname:       host.Spec.Connection.Host,
		Port:           host.Spec.Connection.Port,
		Username:       host.Spec.Connection.User,
		Become:         host.Spec.Privilege.Become,
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

const (
	playbookIndexer = ".spec.playbookRef.name"
)

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleReconcileJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index the playbooks so we know which reconcile jobs to reconcile when they change
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &ansibleoperatorv1alpha1.AnsibleReconcileJob{}, playbookIndexer, func(o client.Object) []string {
		reconcileJob := o.(*ansibleoperatorv1alpha1.AnsibleReconcileJob)
		return []string{reconcileJob.Spec.PlaybookRef.Name}
	})

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ansibleoperatorv1alpha1.AnsibleReconcileJob{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.CronJob{}).
		Watches(&ansibleoperatorv1alpha1.AnsibleHost{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfChangedHostInNamespace)).
		Watches(&ansibleoperatorv1alpha1.AnsibleGroup{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfChangedGroupInNamespace)).
		Watches(&ansibleoperatorv1alpha1.AnsiblePlaybook{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfReferencedPlaybookChanged)).
		Watches(&batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(r.enqueueReconcileJobsIfJobChanged)).
		Named("ansiblereconcilejob").
		Complete(r)
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfJobChanged(ctx context.Context, obj client.Object) []reconcile.Request {
	requests := make([]reconcile.Request, 0, 1)
	jobLabels := obj.GetLabels()
	ownerName, ok := jobLabels[jobLabelOwnerReconcileJob]
	if !ok {
		// Not our job
		return nil
	}
	requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: obj.GetNamespace(), Name: ownerName}})
	return requests
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfReferencedPlaybookChanged(ctx context.Context, obj client.Object) []reconcile.Request {
	var reconcileJobs ansibleoperatorv1alpha1.AnsibleReconcileJobList
	err := r.List(ctx, &reconcileJobs, client.MatchingFields{playbookIndexer: obj.GetName()}, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(reconcileJobs.Items))
	for _, reconcileJob := range reconcileJobs.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&reconcileJob)})
	}
	return requests
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfChangedHostInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	requests, err := r.requestAllReconcileJobsInNamespace(ctx, obj.GetNamespace())
	if err != nil {
		logf.Log.Error(err, "failed to enqueue AnsibleReconcileJobs for AnsibleHost change")
		return []reconcile.Request{}
	}
	return requests
}

func (r *AnsibleReconcileJobReconciler) enqueueReconcileJobsIfChangedGroupInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	requests, err := r.requestAllReconcileJobsInNamespace(ctx, obj.GetNamespace())
	if err != nil {
		logf.Log.Error(err, "failed to enqueue AnsibleReconcileJobs for AnsibleGroup change")
		return []reconcile.Request{}
	}
	return requests
}

func (r *AnsibleReconcileJobReconciler) requestAllReconcileJobsInNamespace(ctx context.Context, namespace string) ([]reconcile.Request, error) {
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
