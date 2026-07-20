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

func AnsibleReconcileJobTests() {
	Describe("AnsibleReconcileJob", Ordered, func() {
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
			By("Cleaning up the test namespace")
			deleteNamespace(testResourceNamespace)
		})

		Context("when created with valid configuration", func() {
			Context("when using an inline playbook", func() {
				It("should successfully apply the playbook", func() {
					By("creating the AnsibleReconcileJob")
					createAnsibleReconcileJob("reconcile-job", testResourceNamespace, "* * * * *", "inline-ansible-playbook")
					By("Waiting for the test files to be created")
					eventuallyFileShouldExist(testResourceNamespace, 0, "/test_file_inline_group.txt")
					eventuallyFileShouldExist(testResourceNamespace, 1, "/test_file_inline_host.txt")
					eventuallyFileShouldExist(testResourceNamespace, 2, "/test_file_inline_host.txt")
					eventuallyFileShouldContain(testResourceNamespace, 0, "/test_file_inline_group.txt", "Hello, group!")
					eventuallyFileShouldContain(testResourceNamespace, 1, "/test_file_inline_host.txt", "Hello, host!")
					eventuallyFileShouldContain(testResourceNamespace, 2, "/test_file_inline_host.txt", "Hello, host!")
				})
			})
			Context("when using a git playbook", func() {
			})
		})
		Context("when created with invalid inventory", func() {
		})
		Context("when created with failing playbook", func() {
		})
	})
}

func eventuallyFileShouldContain(namespace string, node int, filename string, content string) {
	Eventually(func() string {
		return catFileOnSSHNode(namespace, node, filename)
	}).WithTimeout(3 * time.Minute).Should(Equal(content))
}

func eventuallyFileShouldExist(namespace string, node int, filename string) {
	Eventually(func() bool {
		return fileExistsOnSSHNode(namespace, node, filename)
	}).WithTimeout(3 * time.Minute).Should(Equal(true))
}
