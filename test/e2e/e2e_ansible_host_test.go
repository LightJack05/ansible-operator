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

// number of ssh nodes to spawn
const sshNodeCount = 3

const sshNodeNamespace = "ssh-nodes"

func AnsibleHostTests() {
	Context("AnsibleHost", func() {
		It("Should become ready and create a secret when configured correctly", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

			createSSHKeySecret(testNs, "ssh-node-0-credentials")

			createValidAnsibleHost(
				"valid-ansible-host",
				testNs,
				fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", 0, sshNodeNamespace),
				"root",
				"ssh-node-0-credentials",
				"ssh-node-0-hostkey",
				22,
				false,
			)

			By("waiting for the AnsibleHost to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "valid-ansible-host",
					"-n", testNs,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "AnsibleHost not ready")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("waiting for the ansible host to create a secret")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "ssh-node-0-hostkey",
					"-n", testNs,
					"-o", "jsonpath={.data.host_keys}")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)
		})

		It("Should create the AnsibleHost ConfigMap populated with it's variables", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

			createSSHKeySecret(testNs, "ssh-node-0-credentials")

			createValidAnsibleHostWithVars(
				"valid-ansible-host",
				testNs,
				fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", 0, sshNodeNamespace),
				"root",
				"ssh-node-0-credentials",
				"ssh-node-0-hostkey",
				"foobarbaz",
				22,
				false,
			)

			By("waiting for the AnsibleHost to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "valid-ansible-host",
					"-n", testNs,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "AnsibleHost not ready")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("waiting for the ansible host to create a ConfigMap with the variables")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "valid-ansible-host-vars",
					"-n", testNs,
					"-o", "jsonpath={.data.vars}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("foobarbaz"), "AnsibleHost ConfigMap vars not correct")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)

		})

		It("Should become not ready when configured incorrectly, and not create a secret", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

			createSSHKeySecret(testNs, "ssh-node-0-credentials")

			createValidAnsibleHost(
				"ansible-host",
				testNs,
				fmt.Sprintf("invalid-ssh-node-%d.%s.svc.cluster.local", 0, sshNodeNamespace),
				"root",
				"ssh-node-0-credentials",
				"ssh-node-0-hostkey",
				22,
				false,
			)

			By("Waiting for the host to enter not ready state")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "ansible-host",
					"-n", testNs,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"), "AnsibleHost ready, but shouldn't be")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("waiting for the ansible host to not create a secret")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "ssh-node-0-hostkey",
					"-n", testNs,
					"-o", "jsonpath={.data.host_keys}")
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
			}, 30*time.Second, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)
		})

		It("Should fail when there is no private key secret", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

			// Don't create the secret: createSSHKeySecret(testNs, "ssh-node-0-credentials")

			createValidAnsibleHost(
				"valid-ansible-host",
				testNs,
				fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", 0, sshNodeNamespace),
				"root",
				"ssh-node-0-credentials",
				"ssh-node-0-hostkey",
				22,
				false,
			)

			By("waiting for the AnsibleHost to become not ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "valid-ansible-host",
					"-n", testNs,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"), "AnsibleHost ready, but shouldn't be")
			}, 3*time.Minute, time.Second).Should(Succeed())

			// No secret should be created, we don't scan when there is no credential to access the key
			By("waiting for the ansible host to not create a secret")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "ssh-node-0-hostkey",
					"-n", testNs,
					"-o", "jsonpath={.data.host_keys}")
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
			}, 30*time.Second, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)
		})

		It("Should fail when the private key secret is bogus", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

			// Create a bogus secret
			applySSHKeySecret(testNs, "ssh-node-0-credentials", "this-is-not-a-valid-private-key")

			createValidAnsibleHost(
				"valid-ansible-host",
				testNs,
				fmt.Sprintf("ssh-node-%d.%s.svc.cluster.local", 0, sshNodeNamespace),
				"root",
				"ssh-node-0-credentials",
				"ssh-node-0-hostkey",
				22,
				false,
			)

			By("waiting for the AnsibleHost to become not ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "valid-ansible-host",
					"-n", testNs,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"), "AnsibleHost ready, but shouldn't be")
			}, 3*time.Minute, time.Second).Should(Succeed())

			// No secret should be created, we don't scan when there is no credential to access the key
			By("waiting for the ansible host to not create a secret")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "ssh-node-0-hostkey",
					"-n", testNs,
					"-o", "jsonpath={.data.host_keys}")
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
			}, 30*time.Second, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)
		})
	})
}
