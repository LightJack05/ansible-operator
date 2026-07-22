//go:build e2e
// +build e2e

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

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LightJack05/ansible-operator/test/utils"
)

var _ = Describe("AnsibleReconcileJob", func() {
	var testResourceNamespace string
	BeforeEach(func() {
		testResourceNamespace = createRandomTestNamespace()
		By("Creating SSH credential secrets")
		// Created before the SSH nodes so the public keys are in place
		// when the pods mount them as authorized keys.
		for i := range sshNodeCount {
			createSSHKeySecret(testResourceNamespace, fmt.Sprintf("ssh-node-%d-credentials", i))
		}
		By("Setting up 3 SSH hosts...")
		createSSHHosts(testResourceNamespace)
		By("Creating matching AnsibleHost resources")
		for i := range sshNodeCount {
			createValidAnsibleHostWithVars(
				fmt.Sprintf("ansible-host-%d", i),
				testResourceNamespace,
				fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", i, testResourceNamespace),
				"root",
				fmt.Sprintf("ssh-node-%d-credentials", i),
				fmt.Sprintf("ssh-node-%d-hostkey", i),
				`"{host_content: 'Hello, host!'}"`,
				22,
				false,
			)
		}

		By("waiting for all AnsibleHosts to become ready")
		Eventually(func(g Gomega) {
			for i := range sshNodeCount {
				cmd := exec.Command("kubectl", "get", "ansiblehost", fmt.Sprintf("ansible-host-%d", i),
					"-n", testResourceNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), fmt.Sprintf("AnsibleHost ansible-host-%d not ready", i))
			}
		}, 3*time.Minute, time.Second).Should(Succeed())

		By("Creating a valid AnsibleGroup resource")
		var hostNames []string
		for i := range sshNodeCount {
			hostNames = append(hostNames, fmt.Sprintf("ansible-host-%d", i))
		}
		createAnsibleGroup(
			"static-valid-ansible-group",
			testResourceNamespace,
			"{group_content: 'Hello, group!'}",
			hostNames,
			make([]string, 0),
		)
		waitForAnsibleGroupReady("static-valid-ansible-group", testResourceNamespace)

		By("creating a valid ansible playbook pointing at a git repository")
		createGitAnsiblePlaybook(
			"git-ansible-playbook",
			testResourceNamespace,
			"http://git-server.default.svc.cluster.local/git/playbook-repo.git",
			"main",
			"playbook.yml",
			"dir/requirements.yml",
		)

		By("creating a valid ansible playbook with inline playbook and requirements")
		inlinePlaybook := `- name: create file from group vars
  hosts: ansible-host-0
  become: true
  roles:
    - role: ansible_test_role
      vars:
        content: "{{ group_content }}"
        location: /test_file_inline_group.txt

- name: create file from host vars
  hosts: ansible-host-1
  become: true
  roles:
    - role: ansible_test_role
      vars:
        content: "{{ host_content }}"
        location: /test_file_inline_host.txt

- name: create file from host vars with inline task
  hosts: ansible-host-2
  become: true
  tasks:
    - name: Create file
      ansible.builtin.copy:
        content: "{{ host_content }}"
        dest: /test_file_inline_host.txt
`
		inlineRequirements := `roles:
  - name: ansible_test_role
    src: git+http://git-server.default.svc.cluster.local/git/ansible-test-role.git
    version: main
`
		createInlineAnsiblePlaybook(
			"inline-ansible-playbook",
			testResourceNamespace,
			inlinePlaybook,
			inlineRequirements,
		)
	})

	AfterEach(func() {
		dumpControllerLogsOnFailure()
		By("Cleaning up the test namespace")
		deleteNamespace(testResourceNamespace)
	})

	Context("when created with valid configuration", func() {
		Context("when using an inline playbook", func() {
			It("should successfully apply the playbook", func() {
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "inline-ansible-playbook")
				waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Ready", "True")
				By("Waiting for the test files to be created")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_inline_group.txt")
				eventuallyFileShouldExist(testResourceNamespace, 1, "/test_file_inline_host.txt")
				eventuallyFileShouldExist(testResourceNamespace, 2, "/test_file_inline_host.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_inline_group.txt", "Hello, group!")
				eventuallyFileShouldContain(testResourceNamespace, 1, "/test_file_inline_host.txt", "Hello, host!")
				eventuallyFileShouldContain(testResourceNamespace, 2, "/test_file_inline_host.txt", "Hello, host!")
				By("checking whether the status is set to successful")
				waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Successful", "True")
			})
			It("should clean up any cronjobs created when deleted", func() {
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "inline-ansible-playbook")
				By("waiting for the cronjob to be created")
				eventuallyCronjobShouldExist("reconcile-job", testResourceNamespace)
				By("deleting the AnsibleReconcileJob")
				deleteAnsibleReconcileJob("reconcile-job", testResourceNamespace)
				By("waiting for the cronjob to be garbage collected")
				eventuallyCronjobShouldNotExist("reconcile-job", testResourceNamespace)
			})
			It("should apply playbook changes on subsequent runs", func() {
				By("creating an inline ansible playbook writing a marker file")
				createInlineAnsiblePlaybook(
					"update-ansible-playbook",
					testResourceNamespace,
					markerPlaybook("first version"),
					"",
				)
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "update-ansible-playbook")
				By("waiting for the marker file to be created with the initial content")
				eventuallyFileShouldExist(testResourceNamespace, 2, "/test_file_update.txt")
				eventuallyFileShouldContain(testResourceNamespace, 2, "/test_file_update.txt", "first version")
				By("updating the inline playbook")
				createInlineAnsiblePlaybook(
					"update-ansible-playbook",
					testResourceNamespace,
					markerPlaybook("second version"),
					"",
				)
				By("waiting for the marker file to be updated on a subsequent run")
				eventuallyFileShouldContain(testResourceNamespace, 2, "/test_file_update.txt", "second version")
			})
			It("should apply updated host and group vars on subsequent runs", func() {
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "inline-ansible-playbook")
				By("waiting for the test files to be created with the initial vars")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_inline_group.txt")
				eventuallyFileShouldExist(testResourceNamespace, 1, "/test_file_inline_host.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_inline_group.txt", "Hello, group!")
				eventuallyFileShouldContain(testResourceNamespace, 1, "/test_file_inline_host.txt", "Hello, host!")
				By("updating the group vars")
				createAnsibleGroup(
					"static-valid-ansible-group",
					testResourceNamespace,
					"{group_content: 'Updated, group!'}",
					[]string{"ansible-host-0", "ansible-host-1", "ansible-host-2"},
					make([]string, 0),
				)
				By("updating the host vars of ansible-host-1")
				createValidAnsibleHostWithVars(
					"ansible-host-1",
					testResourceNamespace,
					fmt.Sprintf("ssh-node-1.%s.svc.cluster.local", testResourceNamespace),
					"root",
					"ssh-node-1-credentials",
					"ssh-node-1-hostkey",
					`"{host_content: 'Updated, host!'}"`,
					22,
					false,
				)
				By("waiting for the test files to be updated on a subsequent run")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_inline_group.txt", "Updated, group!")
				eventuallyFileShouldContain(testResourceNamespace, 1, "/test_file_inline_host.txt", "Updated, host!")
			})
		})
		Context("when using a git playbook", func() {
			It("should successfully apply the playbook", func() {
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "git-ansible-playbook")
				waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Ready", "True")
				By("Waiting for the test files to be created")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_group.txt")
				eventuallyFileShouldExist(testResourceNamespace, 1, "/test_file_host.txt")
				eventuallyFileShouldExist(testResourceNamespace, 2, "/test_file_host.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_group.txt", "Hello, group!")
				eventuallyFileShouldContain(testResourceNamespace, 1, "/test_file_host.txt", "Hello, host!")
				eventuallyFileShouldContain(testResourceNamespace, 2, "/test_file_host.txt", "Hello, host!")
				By("checking whether the status is set to successful")
				waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Successful", "True")
			})
			It("should use the new playbook when the referenced playbook is switched from git to inline", func() {
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "git-ansible-playbook")
				By("waiting for the git playbook to be applied")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_group.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_group.txt", "Hello, group!")
				By("switching the referenced playbook from git to inline")
				switchedPlaybook := `- name: write marker file after switching to inline
  hosts: ansible-host-0
  become: true
  tasks:
    - name: Create marker file
      ansible.builtin.copy:
        content: switched to inline
        dest: /test_file_switched.txt
`
				createInlineAnsiblePlaybook(
					"git-ansible-playbook",
					testResourceNamespace,
					switchedPlaybook,
					"",
				)
				By("waiting for the inline playbook to be applied on a subsequent run")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_switched.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_switched.txt", "switched to inline")
			})
			It("should use the new playbook when the referenced playbook is switched from inline to git", func() {
				// This guards against stale inline keys in the runtime
				// configmap shadowing the git checkout after the switch.
				By("creating an inline playbook writing a marker file")
				inlineBeforeSwitch := `- name: write marker file before switching to git
  hosts: ansible-host-0
  become: true
  tasks:
    - name: Create marker file
      ansible.builtin.copy:
        content: inline before switch
        dest: /test_file_before_switch.txt
`
				createInlineAnsiblePlaybook(
					"switching-playbook",
					testResourceNamespace,
					inlineBeforeSwitch,
					"",
				)
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "switching-playbook")
				By("waiting for the inline playbook to be applied")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_before_switch.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_before_switch.txt", "inline before switch")
				By("switching the referenced playbook from inline to git")
				createGitAnsiblePlaybook(
					"switching-playbook",
					testResourceNamespace,
					"http://git-server.default.svc.cluster.local/git/playbook-repo.git",
					"main",
					"playbook.yml",
					"dir/requirements.yml",
				)
				By("waiting for the git playbook to be applied on a subsequent run")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_group.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_group.txt", "Hello, group!")
			})
		})
		Context("when a host is added after the reconcile job is created", func() {
			It("should include the new host in subsequent runs", func() {
				By("creating an inline ansible playbook targeting a host that does not exist yet")
				extraHostPlaybook := `- name: write file on the extra host
  hosts: ansible-host-extra
  become: true
  tasks:
    - name: Create file
      ansible.builtin.copy:
        content: "{{ extra_content }}"
        dest: /test_file_extra.txt
`
				createInlineAnsiblePlaybook(
					"extra-host-playbook",
					testResourceNamespace,
					extraHostPlaybook,
					"",
				)
				By("creating the AnsibleReconcileJob")
				createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "extra-host-playbook")
				By("waiting for a run to complete without the extra host")
				waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Successful", "True")
				Expect(fileExistsOnSSHNode(testResourceNamespace, 0, "/test_file_extra.txt")).To(BeFalse(),
					"the extra host file should not exist before the extra host is created")
				By("creating the extra AnsibleHost pointing at the first SSH node")
				createValidAnsibleHostWithVars(
					"ansible-host-extra",
					testResourceNamespace,
					fmt.Sprintf("ssh-node-0.%s.svc.cluster.local", testResourceNamespace),
					"root",
					"ssh-node-0-credentials",
					"ansible-host-extra-hostkey",
					`"{extra_content: 'Hello, extra!'}"`,
					22,
					false,
				)
				By("waiting for the extra AnsibleHost to become ready")
				waitForAnsibleHostReady("ansible-host-extra", testResourceNamespace)
				By("waiting for the extra host to be picked up on a subsequent run")
				eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_extra.txt")
				eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_extra.txt", "Hello, extra!")
			})
		})
	})
	Context("when created with invalid inventory", func() {
		It("should report not ready if there is an unresolved reference", func() {
			By("Creating an unresolving ansible group")
			createAnsibleGroup("invalid", testResourceNamespace, "", []string{"absent"}, nil)
			By("creating the ansible reconcile job")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "git-ansible-playbook")
			By("Waiting for the ready status on the resource to become false")
			waitForAnsibleReconcileJobNotReady("reconcile-job", testResourceNamespace)
		})
		It("should pause the cronjob created when the inventory becomes invalid after it is created.", func() {
			By("creating the ansible reconcile job with a valid inventory")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "git-ansible-playbook")
			By("waiting for the cronjob to be created and not suspended")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, false)
			By("making the inventory invalid by creating an unresolving ansible group")
			createAnsibleGroup("invalid", testResourceNamespace, "", []string{"absent"}, nil)
			By("waiting for the ready status on the resource to become false")
			waitForAnsibleReconcileJobNotReady("reconcile-job", testResourceNamespace)
			By("waiting for the cronjob to be suspended")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, true)
		})
		It("should resume the cronjob when the inventory becomes valid again", func() {
			By("creating the ansible reconcile job with a valid inventory")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "git-ansible-playbook")
			By("waiting for the cronjob to be created and not suspended")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, false)
			By("making the inventory invalid by creating an unresolving ansible group")
			createAnsibleGroup("invalid", testResourceNamespace, "", []string{"absent"}, nil)
			By("waiting for the cronjob to be suspended")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, true)
			By("deleting the unresolving ansible group")
			deleteAnsibleGroup("invalid", testResourceNamespace)
			By("waiting for the ready status on the resource to become true again")
			waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Ready", "True")
			By("waiting for the cronjob to be resumed")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, false)
		})
	})
	Context("when the referenced playbook is deleted", func() {
		It("should report not ready and suspend the cronjob", func() {
			By("creating the AnsibleReconcileJob")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "inline-ansible-playbook")
			By("waiting for the resource to become ready and the cronjob to be created")
			waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Ready", "True")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, false)
			By("deleting the referenced playbook")
			deleteAnsiblePlaybook("inline-ansible-playbook", testResourceNamespace)
			By("waiting for the ready status on the resource to become false")
			waitForAnsibleReconcileJobNotReady("reconcile-job", testResourceNamespace)
			By("waiting for the cronjob to be suspended")
			eventuallyCronjobSuspendShouldBe("reconcile-job", testResourceNamespace, true)
		})
	})
	Context("when created with a nonexistent playbook reference", func() {
		It("should report not ready and not create a cronjob", func() {
			By("creating the AnsibleReconcileJob referencing a playbook that does not exist")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "nonexistent-playbook")
			By("waiting for the ready status on the resource to become false")
			waitForAnsibleReconcileJobNotReady("reconcile-job", testResourceNamespace)
			By("checking that no cronjob was created")
			eventuallyCronjobShouldNotExist("reconcile-job", testResourceNamespace)
		})
	})
	Context("when created with an invalid schedule", func() {
		It("should report not ready and not create a cronjob", func() {
			By("creating the AnsibleReconcileJob with an invalid cron schedule")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "not-a-schedule", "inline-ansible-playbook")
			By("waiting for the ready status on the resource to become false")
			waitForAnsibleReconcileJobNotReady("reconcile-job", testResourceNamespace)
			By("checking that no cronjob was created")
			eventuallyCronjobShouldNotExist("reconcile-job", testResourceNamespace)
		})
	})
	Context("when created with failing playbook", func() {
		It("should eventually report the success status as false", func() {
			By("creating an inline ansible playbook that always fails")
			failingPlaybook := `- name: always fail
  hosts: ansible-host-0
  tasks:
    - name: Fail
      ansible.builtin.fail:
        msg: This playbook is expected to fail
`
			createInlineAnsiblePlaybook(
				"failing-ansible-playbook",
				testResourceNamespace,
				failingPlaybook,
				"",
			)
			By("creating the AnsibleReconcileJob")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "failing-ansible-playbook")
			By("waiting for the Successful condition on the resource to become false")
			waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Successful", "False")
		})
		It("should eventually report the success status as false when the git repository cannot be fetched", func() {
			By("creating a git ansible playbook pointing at a nonexistent repository")
			createGitAnsiblePlaybook(
				"broken-git-playbook",
				testResourceNamespace,
				"http://git-server.default.svc.cluster.local/git/nonexistent-repo.git",
				"main",
				"playbook.yml",
				"",
			)
			By("creating the AnsibleReconcileJob")
			createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "broken-git-playbook")
			By("waiting for the Successful condition on the resource to become false")
			waitForAnsibleReconcileJobConditionStatus("reconcile-job", testResourceNamespace, "Successful", "False")
		})
	})
})

// markerPlaybook returns an inline playbook that writes the given content to
// /test_file_update.txt on ansible-host-2.
func markerPlaybook(content string) string {
	return fmt.Sprintf(`- name: write marker file
  hosts: ansible-host-2
  become: true
  tasks:
    - name: Create marker file
      ansible.builtin.copy:
        content: %s
        dest: /test_file_update.txt
`, content)
}

func eventuallyFileShouldContain(namespace string, node int, filename string, content string) {
	GinkgoHelper()
	Eventually(func() string {
		return catFileOnSSHNode(namespace, node, filename)
	}).WithTimeout(3 * time.Minute).Should(Equal(content))
}

func eventuallyFileShouldExist(namespace string, node int, filename string) {
	GinkgoHelper()
	Eventually(func() bool {
		return fileExistsOnSSHNode(namespace, node, filename)
	}).WithTimeout(3 * time.Minute).Should(Equal(true))
}

func waitForAnsibleReconcileJobNotReady(name, namespace string) {
	GinkgoHelper()
	waitForAnsibleReconcileJobConditionStatus(name, namespace, "Ready", "False")
}

func waitForAnsibleReconcileJobConditionStatus(name, namespace, conditionType, status string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblereconcilejob", name,
			"-n", namespace,
			"-o", fmt.Sprintf("jsonpath={.status.conditions[?(@.type=='%s')].status}", conditionType))
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(status),
			fmt.Sprintf("AnsibleReconcileJob %s condition %s did not become %s", name, conditionType, status))
	}, 3*time.Minute, time.Second).Should(Succeed())
}

func eventuallyCronjobShouldExist(name, namespace string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "cronjob", name, "-n", namespace,
			"-o", "jsonpath={.metadata.name}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(name))
	}, 3*time.Minute, time.Second).Should(Succeed())
}

func eventuallyCronjobShouldNotExist(name, namespace string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "cronjob", name, "-n", namespace,
			"--ignore-not-found", "-o", "jsonpath={.metadata.name}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(BeEmpty(), fmt.Sprintf("cronjob %s still exists", name))
	}, 3*time.Minute, time.Second).Should(Succeed())
}

func eventuallyCronjobSuspendShouldBe(name, namespace string, suspended bool) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "cronjob", name, "-n", namespace,
			"-o", "jsonpath={.spec.suspend}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(fmt.Sprintf("%v", suspended)),
			fmt.Sprintf("cronjob %s suspend flag did not become %v", name, suspended))
	}, 3*time.Minute, time.Second).Should(Succeed())
}
