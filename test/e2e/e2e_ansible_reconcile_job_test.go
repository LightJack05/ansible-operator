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
			By("Setting up 3 SSH hosts...")
			createSSHHosts(testResourceNamespace)
			By("Creating matching AnsibleHost resources")
			for i := range sshNodeCount {
				createSSHKeySecret(testResourceNamespace, fmt.Sprintf("ssh-node-%d-credentials", i))
				createValidAnsibleHost(
					fmt.Sprintf("ansible-host-%d", i),
					testResourceNamespace,
					fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", i, testResourceNamespace),
					"root",
					fmt.Sprintf("ssh-node-%d-credentials", i),
					fmt.Sprintf("ssh-node-%d-hostkey", i),
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
				"",
				hostNames,
				make([]string, 0),
			)
			waitForAnsibleGroupReady("static-valid-ansible-group", testResourceNamespace)
		})

		AfterEach(func() {
			By("Cleaning up the test namespace")
			deleteNamespace(testResourceNamespace)
		})

		Context("when created with valid configuration", func() {
		})
		Context("when created with invalid inventory", func() {
		})
		Context("when created with failing playbook", func() {
		})
	})
}
