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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ansibleoperatorv1alpha1 "github.com/LightJack05/ansible-operator/api/v1alpha1"
)

// AnsibleGroupReconciler reconciles a AnsibleGroup object
type AnsibleGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type groupHealthcheckResult struct {
	subgroupsNotFound   []v1.LocalObjectReference
	subgroupsNotHealthy []v1.LocalObjectReference
	hostsNotFound       []v1.LocalObjectReference
	hostsNotHealthy     []v1.LocalObjectReference
}

// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ansible-operator.lightjack.de,resources=ansiblegroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AnsibleGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AnsibleGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := logf.FromContext(ctx)

	// Fetch the AnsibleGroup instance
	var group ansibleoperatorv1alpha1.AnsibleGroup
	if err := r.Get(ctx, req.NamespacedName, &group); err != nil {
		if errors.IsNotFound(err) {
			lg.Info("AnsibleGroup resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		lg.Error(err, "Failed to get AnsibleGroup")
		return ctrl.Result{}, err
	}

	// Set all conditions to unknown if they are not set
	if err := r.defaultStatusToUnknown(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionHealthy); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
	}
	if err := r.defaultStatusToUnknown(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReferencesValid); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
	}
	if err := r.defaultStatusToUnknown(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReady); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
	}

	// Check the health of the group by verifying the existence and health of referenced hosts and subgroups
	healthResult, err := r.checkGroupHealth(ctx, &group)
	if err != nil {
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReady, metav1.ConditionFalse, "Error", fmt.Sprintf("Failed to check group health: %v", err)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
		lg.Error(err, "Failed to check group health")
		return ctrl.Result{}, err
	}

	refsInvalid := len(healthResult.subgroupsNotFound) > 0 || len(healthResult.hostsNotFound) > 0
	objectsUnhealthy := len(healthResult.subgroupsNotHealthy) > 0 || len(healthResult.hostsNotHealthy) > 0

	if !refsInvalid && !objectsUnhealthy {
		// If everything is healthy, set the conditions to True and return
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionHealthy, metav1.ConditionTrue, "AllHealthy", "All referenced hosts and subgroups are healthy"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReferencesValid, metav1.ConditionTrue, "AllValid", "All referenced hosts and subgroups are valid"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReady, metav1.ConditionTrue, "Ready", "AnsibleGroup is ready"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
		return ctrl.Result{}, nil
	}

	var invalidReferenceStringBuilder strings.Builder
	if refsInvalid {
		invalidReferenceStringBuilder.WriteString("Missing Hosts: ")
		invalidReferenceStringBuilder.WriteString(objectReferenceListToString(healthResult.hostsNotFound))
		invalidReferenceStringBuilder.WriteString(";")

		invalidReferenceStringBuilder.WriteString("Missing Subgroups: ")
		invalidReferenceStringBuilder.WriteString(objectReferenceListToString(healthResult.subgroupsNotFound))
		invalidReferenceStringBuilder.WriteString(";")

		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReferencesValid, metav1.ConditionFalse, "InvalidReferences", fmt.Sprintf("One or more references are invalid: %s", invalidReferenceStringBuilder.String())); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	} else {
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReferencesValid, metav1.ConditionTrue, "AllValid", "All referenced hosts and subgroups are valid"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	}

	var unhealthyMessageStringBuilder strings.Builder
	if objectsUnhealthy {
		unhealthyMessageStringBuilder.WriteString("Unhealthy Hosts: ")
		unhealthyMessageStringBuilder.WriteString(objectReferenceListToString(healthResult.hostsNotHealthy))
		unhealthyMessageStringBuilder.WriteString(";")

		unhealthyMessageStringBuilder.WriteString("Unhealthy Subgroups: ")
		unhealthyMessageStringBuilder.WriteString(objectReferenceListToString(healthResult.subgroupsNotHealthy))
		unhealthyMessageStringBuilder.WriteString(";")

		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionHealthy, metav1.ConditionFalse, "UnhealthyReferences", fmt.Sprintf("One or more referenced objects are unhealthy: %s", unhealthyMessageStringBuilder.String())); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	} else {
		if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionHealthy, metav1.ConditionTrue, "AllHealthy", "All referenced hosts and subgroups are healthy"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	}

	if err := r.setCondition(ctx, &group, ansibleoperatorv1alpha1.AnsibleGroupConditionReady, metav1.ConditionFalse, "UnhealthyOrInvalidReferences", "One or more references are invalid or unhealthy."); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
	}

	return ctrl.Result{}, nil
}

func objectReferenceListToString(refs []v1.LocalObjectReference) string {
	var names strings.Builder
	for _, ref := range refs {
		names.WriteString(ref.Name)
		names.WriteString(",")
	}
	return names.String()
}

func (r *AnsibleGroupReconciler) setCondition(ctx context.Context, group *ansibleoperatorv1alpha1.AnsibleGroup, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) error {
	changed := meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})

	if changed {
		if err := r.Status().Update(ctx, group); err != nil {
			return fmt.Errorf("unable to update AnsibleGroup status: %v", err)
		}
	}

	return nil
}

func (r *AnsibleGroupReconciler) checkGroupHealth(ctx context.Context, group *ansibleoperatorv1alpha1.AnsibleGroup) (*groupHealthcheckResult, error) {
	result := &groupHealthcheckResult{}

	missingGroups, unhealthyGroups, err := r.isGroupsArrayOk(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("error checking groups: %v", err)
	}
	result.subgroupsNotFound = missingGroups
	result.subgroupsNotHealthy = unhealthyGroups

	missingHosts, unhealthyHosts, err := r.isHostsArrayOk(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("error checking hosts: %v", err)
	}
	result.hostsNotFound = missingHosts
	result.hostsNotHealthy = unhealthyHosts

	return result, nil
}

func (r *AnsibleGroupReconciler) isGroupsArrayOk(ctx context.Context, group *ansibleoperatorv1alpha1.AnsibleGroup) (missingGroups, unhealthyGroups []v1.LocalObjectReference, err error) {
	// Rely on the cache from kubebuilder for the API requests, this should be nearly O(1) lookups and should only do one List already
	for _, subgroup := range group.Spec.Groups {
		var subgroupObject ansibleoperatorv1alpha1.AnsibleGroup
		err := r.Get(ctx, client.ObjectKey{Namespace: group.Namespace, Name: subgroup.Name}, &subgroupObject)
		if errors.IsNotFound(err) {
			missingGroups = append(missingGroups, subgroup)
		} else if err != nil {
			return nil, nil, fmt.Errorf("unable to check health for object '%s': %v", subgroup.Name, err)
		} else if condition := meta.FindStatusCondition(subgroupObject.Status.Conditions, "Ready"); condition == nil || condition.Status != metav1.ConditionTrue {
			unhealthyGroups = append(unhealthyGroups, subgroup)
		}
	}

	return missingGroups, unhealthyGroups, nil
}

func (r *AnsibleGroupReconciler) isHostsArrayOk(ctx context.Context, group *ansibleoperatorv1alpha1.AnsibleGroup) (missingHosts, unhealthyHosts []v1.LocalObjectReference, err error) {
	// Rely on the cache from kubebuilder for the API requests, this should be nearly O(1) lookups and should only do one List already
	for _, host := range group.Spec.Hosts {
		var hostObject ansibleoperatorv1alpha1.AnsibleHost
		err := r.Get(ctx, client.ObjectKey{Namespace: group.Namespace, Name: host.Name}, &hostObject)
		if errors.IsNotFound(err) {
			missingHosts = append(missingHosts, host)
		} else if err != nil {
			return nil, nil, fmt.Errorf("unable to check health for object '%s': %v", host.Name, err)
		} else if condition := meta.FindStatusCondition(hostObject.Status.Conditions, "Ready"); condition == nil || condition.Status != metav1.ConditionTrue {
			unhealthyHosts = append(unhealthyHosts, host)
		}
	}

	return missingHosts, unhealthyHosts, nil
}

const (
	hostIndex   = ".spec.hosts"
	groupsIndex = ".spec.groups"
)

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index: Subgroups
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &ansibleoperatorv1alpha1.AnsibleGroup{}, groupsIndex,
		func(o client.Object) []string {
			group := o.(*ansibleoperatorv1alpha1.AnsibleGroup)
			childGroups := make([]string, 0, len(group.Spec.Groups))
			for _, childGroup := range group.Spec.Groups {
				childGroups = append(childGroups, childGroup.Name)
			}
			return childGroups
		})

	if err != nil {
		return err
	}

	// Index: Hosts

	err = mgr.GetFieldIndexer().IndexField(context.Background(), &ansibleoperatorv1alpha1.AnsibleGroup{}, hostIndex,
		func(o client.Object) []string {
			group := o.(*ansibleoperatorv1alpha1.AnsibleGroup)
			childHosts := make([]string, 0, len(group.Spec.Hosts))
			for _, childHost := range group.Spec.Hosts {
				childHosts = append(childHosts, childHost.Name)
			}
			return childHosts
		})

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ansibleoperatorv1alpha1.AnsibleGroup{}).
		Named("ansiblegroup").
		Watches(&ansibleoperatorv1alpha1.AnsibleHost{}, handler.EnqueueRequestsFromMapFunc(r.requestsForHostChange)).
		Watches(&ansibleoperatorv1alpha1.AnsibleGroup{}, handler.EnqueueRequestsFromMapFunc(r.requestsForGroupChange)).
		Complete(r)
}

func (r *AnsibleGroupReconciler) requestsForGroupChange(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.groupsMatchingIndexer(ctx, groupsIndex, obj)
}

func (r *AnsibleGroupReconciler) requestsForHostChange(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.groupsMatchingIndexer(ctx, hostIndex, obj)
}

func (r *AnsibleGroupReconciler) defaultStatusToUnknown(ctx context.Context, group *ansibleoperatorv1alpha1.AnsibleGroup, status string) error {
	if meta.FindStatusCondition(group.Status.Conditions, status) == nil {
		if err := r.setCondition(ctx, group, status, metav1.ConditionUnknown, "ReconcileStarted", fmt.Sprintf("AnsibleGroup is initializing: %s condition not set", status)); err != nil {
			return fmt.Errorf("failed to set condition on AnsibleGroup: %v", err)
		}
	}
	return nil
}

func (r *AnsibleGroupReconciler) groupsMatchingIndexer(ctx context.Context, index string, obj client.Object) []reconcile.Request {
	var groups ansibleoperatorv1alpha1.AnsibleGroupList
	err := r.List(ctx, &groups, client.MatchingFields{index: obj.GetName()}, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(groups.Items))
	for _, group := range groups.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&group)})
	}
	return requests
}
