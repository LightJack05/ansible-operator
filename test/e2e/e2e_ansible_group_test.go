//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LightJack05/ansible-operator/test/utils"
)

func AnsibleGroupTests() {
	Describe("AnsibleGroup", Ordered, func() {
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

			By("Creating an AnsibleHost that will not become ready")
			createValidAnsibleHost(
				"unready-ansible-host",
				testResourceNamespace,
				"nonexistant-host.ssh-nodes.svc.cluster.local",
				"root",
				"ssh-node-X-credentials",
				"ssh-node-X-hostkey",
				22,
				false,
			)

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

			By("Waiting for the unready AnsibleHost to become not ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "ansiblehost", "unready-ansible-host",
					"-n", testResourceNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"), "Unready AnsibleHost is not not ready")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("Creating an AnsibleGroup that will not become ready")
			createAnsibleGroup(
				"unready-ansible-group",
				testResourceNamespace,
				"",
				[]string{"nonexistant-host"},
				make([]string, 0),
			)

			By("Waiting for the unready AnsibleGroup to become not ready")
			waitForAnsibleGroupNotReady("unready-ansible-group", testResourceNamespace)

		})

		AfterEach(func() {
			By("Cleaning up the test namespace")
			deleteNamespace(testResourceNamespace)
		})

		Context("when created with valid configuration", func() {
			It("should become ready when only hosts are specified", func() {
				By("Creating an ansible group")
				createAnsibleGroup(
					"valid-ansible-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "ansible-host-1", "ansible-host-2"},
					make([]string, 0),
				)
				By("waiting for the AnsibleGroup to become ready")
				waitForAnsibleGroupReady("valid-ansible-group", testResourceNamespace)
			})
			It("should become ready when only child groups are specified", func() {
				By("Creating an ansible group that has the default group as a child")
				createAnsibleGroup(
					"parent-group",
					testResourceNamespace,
					"",
					make([]string, 0),
					[]string{"static-valid-ansible-group"},
				)
				By("waiting for the AnsibleGroup to become ready")
				waitForAnsibleGroupReady("parent-group", testResourceNamespace)
			})
			It("should become ready when both hosts and child groups are specified", func() {
				By("Creating an ansible group that has the default group as a child and also has hosts")
				createAnsibleGroup(
					"mixed-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "ansible-host-1"},
					[]string{"static-valid-ansible-group"},
				)
				By("waiting for the AnsibleGroup to become ready")
				waitForAnsibleGroupReady("mixed-group", testResourceNamespace)
			})
		})
		Context("when created with nonexistant refs in configuration", func() {
			It("should become not ready when it references a nonexistant host", func() {
				By("Creating an ansible group that references a nonexistant host")
				createAnsibleGroup(
					"invalid-host-ref-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "nonexistant-host"},
					make([]string, 0),
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("invalid-host-ref-group", testResourceNamespace)
				referencesValidStatusOnAnsibleGroupShouldContain("invalid-host-ref-group", testResourceNamespace, "nonexistant-host")
				referencesValidStatusOnAnsibleGroupShouldHaveReason("invalid-host-ref-group", testResourceNamespace, "InvalidReferences")
			})
			It("should become not ready when it references a nonexistant child group", func() {
				By("Creating an ansible group that references a nonexistant child group")
				createAnsibleGroup(
					"invalid-child-group-ref-group",
					testResourceNamespace,
					"",
					make([]string, 0),
					[]string{"static-valid-ansible-group", "nonexistant-child-group"},
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("invalid-child-group-ref-group", testResourceNamespace)
				referencesValidStatusOnAnsibleGroupShouldContain("invalid-child-group-ref-group", testResourceNamespace, "nonexistant-child-group")
				referencesValidStatusOnAnsibleGroupShouldHaveReason("invalid-child-group-ref-group", testResourceNamespace, "InvalidReferences")
			})
			It("should become not ready when it references a nonexistant host and a nonexistant child group", func() {
				By("Creating an ansible group that references a nonexistant host and a nonexistant child group")
				createAnsibleGroup(
					"invalid-host-and-child-group-ref-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "nonexistant-host"},
					[]string{"static-valid-ansible-group", "nonexistant-child-group"},
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("invalid-host-and-child-group-ref-group", testResourceNamespace)
				referencesValidStatusOnAnsibleGroupShouldContain("invalid-host-and-child-group-ref-group", testResourceNamespace, "nonexistant-host")
				referencesValidStatusOnAnsibleGroupShouldContain("invalid-host-and-child-group-ref-group", testResourceNamespace, "nonexistant-child-group")
				referencesValidStatusOnAnsibleGroupShouldHaveReason("invalid-host-and-child-group-ref-group", testResourceNamespace, "InvalidReferences")
				readyStatusOnAnsibleGroupShouldHaveReason("invalid-host-and-child-group-ref-group", testResourceNamespace, "InvalidReferences")
			})
		})
		Context("when created with unready child resources", func() {
			It("should become not ready when it has an unready host", func() {
				By("Creating an ansible group that references an unready host")
				createAnsibleGroup(
					"unready-host-ref-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "unready-ansible-host"},
					make([]string, 0),
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("unready-host-ref-group", testResourceNamespace)
				readyStatusOnAnsibleGroupShouldContain("unready-host-ref-group", testResourceNamespace, "unready-ansible-host")
				readyStatusOnAnsibleGroupShouldHaveReason("unready-host-ref-group", testResourceNamespace, "UnhealthyReferences")
			})
			It("should become not ready when it has an unready child group", func() {
				By("Creating an ansible group that references an unready child group")
				createAnsibleGroup(
					"unready-child-group-ref-group",
					testResourceNamespace,
					"",
					make([]string, 0),
					[]string{"static-valid-ansible-group", "unready-ansible-group"},
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("unready-child-group-ref-group", testResourceNamespace)
				readyStatusOnAnsibleGroupShouldContain("unready-child-group-ref-group", testResourceNamespace, "unready-ansible-group")
				readyStatusOnAnsibleGroupShouldHaveReason("unready-child-group-ref-group", testResourceNamespace, "UnhealthyReferences")
			})
			It("should become not ready when it has an unready host and an unready child group", func() {
				By("Creating an ansible group that references an unready host and an unready child group")
				createAnsibleGroup(
					"unready-host-and-child-group-ref-group",
					testResourceNamespace,
					"",
					[]string{"ansible-host-0", "unready-ansible-host"},
					[]string{"static-valid-ansible-group", "unready-ansible-group"},
				)
				By("waiting for the AnsibleGroup to become not ready")
				waitForAnsibleGroupNotReady("unready-host-and-child-group-ref-group", testResourceNamespace)
				readyStatusOnAnsibleGroupShouldContain("unready-host-and-child-group-ref-group", testResourceNamespace, "unready-ansible-host")
				readyStatusOnAnsibleGroupShouldContain("unready-host-and-child-group-ref-group", testResourceNamespace, "unready-ansible-group")
				readyStatusOnAnsibleGroupShouldHaveReason("unready-host-and-child-group-ref-group", testResourceNamespace, "UnhealthyReferences")
			})
		})
	})
}

func referencesValidStatusOnAnsibleGroupShouldContain(name, namespace, expectedStatus string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='ReferencesValid')].message}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(ContainSubstring(expectedStatus), fmt.Sprintf("AnsibleGroup Ready status does not contain %s", expectedStatus))
	}, 3*time.Minute, time.Second).Should(Succeed())
}
func referencesValidStatusOnAnsibleGroupShouldHaveReason(name, namespace, expectedReason string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='ReferencesValid')].reason}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(expectedReason), fmt.Sprintf("AnsibleGroup Ready status reason is not %s", expectedReason))
	}, 3*time.Minute, time.Second).Should(Succeed())
}
func readyStatusOnAnsibleGroupShouldContain(name, namespace, expectedStatus string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].message}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(ContainSubstring(expectedStatus), fmt.Sprintf("AnsibleGroup Ready status does not contain %s", expectedStatus))
	}, 3*time.Minute, time.Second).Should(Succeed())
}
func readyStatusOnAnsibleGroupShouldHaveReason(name, namespace, expectedReason string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].reason}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(expectedReason), fmt.Sprintf("AnsibleGroup Ready status reason is not %s", expectedReason))
	}, 3*time.Minute, time.Second).Should(Succeed())
}

func waitForAnsibleGroupReady(name, namespace string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("True"), "AnsibleGroup not ready")
	}, 3*time.Minute, time.Second).Should(Succeed())
}

func waitForAnsibleGroupNotReady(name, namespace string) {
	GinkgoHelper()
	Consistently(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "ansiblegroup", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("True"), "AnsibleGroup not not ready")
	}, 30*time.Second, time.Second).ShouldNot(Succeed())
}
