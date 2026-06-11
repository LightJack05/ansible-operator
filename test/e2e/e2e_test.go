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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LightJack05/ansible-operator/test/utils"
)

// number of ssh nodes to spawn
const sshNodeCount = 3

// namespace where the project is deployed in
const namespace = "ansible-operator-system"

const sshNodeNamespace = "ssh-nodes"

// serviceAccountName created for the project
const serviceAccountName = "ansible-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "ansible-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "ansible-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("deploying the ssh nodes")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=ansible-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": [
								"for i in $(seq 1 30); do curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics && exit 0 || sleep 2; done; exit 1"
							],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

	})

	Context("AnsibleHost", func() {
		It("Should become ready and create a secret when configured correctly", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

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

		It("Should become not ready when configured incorrectly, and not create a secret", func() {
			By("Setting up 3 SSH hosts...")
			setupSSHHosts(sshNodeNamespace)

			By("Creating a test namespace and a valid AnsibleHost resource")
			testNs := createRandomTestNamespace()

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
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("cleaning up the test namespace")
			deleteNamespace(testNs)

			By("removing the existing SSH hosts again")
			deleteNamespace(sshNodeNamespace)
		})
	})
})

func randomHex(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	Expect(err).NotTo(HaveOccurred())
	return hex.EncodeToString(b)
}

func deleteNamespace(name string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "delete", "namespace", name)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	namespaceList := func(g Gomega) {
		stdout, err := utils.Run(exec.Command("kubectl", "get", "ns"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(stdout).ShouldNot(ContainSubstring(name))
	}

	Eventually(namespaceList).Should(Succeed())
}

func createRandomTestNamespace() string {
	GinkgoHelper()
	randomSuffix := randomHex(10)
	namespace := fmt.Sprintf("test-namespace-%s", randomSuffix)
	createNamespace(namespace)
	return namespace
}

func createValidAnsibleHost(name, namespace, hostname, username, privateKeySecretName, hostKeySecretName string, port int, ignoreHostKeys bool) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateAnsibleHost(
		name,
		namespace,
		hostname,
		username,
		privateKeySecretName,
		hostKeySecretName,
		port,
		ignoreHostKeys,
	))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsibleHost")
}

func templateAnsibleHost(name, namespace, hostname, username, privateKeySecretName, hostKeySecretName string, port int, ignoreHostKeys bool) string {
	return fmt.Sprintf(
		`
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleHost
metadata:
  name: %[1]s 
  namespace: %[3]s 
spec:
  connection:
    host: %[2]s
    port: %[7]d
    user: %[4]s
  ssh:
    sshKeySecretRef:
      name: %[5]s
    sshHostKeySecretRef:
      name: %[6]s
    ignoreHostKey: %[8]v
  privilege:
    become: true
---
`,
		name, hostname, namespace, username, privateKeySecretName, hostKeySecretName, port, ignoreHostKeys)
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

func setupSSHHosts(sshNodesNamespace string) {
	GinkgoHelper()
	createNamespace(sshNodesNamespace)
	var manifests strings.Builder
	for i := range sshNodeCount {
		manifests.WriteString(templateSSHServer(sshNodesNamespace, i))
	}

	fmt.Println(manifests.String())
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(manifests.String())
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create admin credentials Secret")

	By("waiting for SSH Node Deployments to be available")

	for i := range sshNodeCount {
		By(fmt.Sprintf("waiting for SSH Node Deployment ssh-node-%d to be available", i))
		cmd := exec.Command("kubectl", "wait", fmt.Sprintf("deployments/ssh-node-%d", i),
			"--namespace", sshNodeNamespace,
			"--for=condition=Available",
			"--timeout=5m",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "SSH deployment did not become available")
	}
}

func createNamespace(name string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "create", "namespace", name)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func templateSSHServer(nodeNamespace string, nodeId int) string {
	return fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ssh-node-%[2]d
  namespace: %[1]s
spec:
  selector:
    matchLabels:
      app: ssh-node-%[2]d
  template:
    metadata:
      labels:
        app: ssh-node-%[2]d
    spec:
      containers:
      - name: ssh-node-%[2]d
        image: localhost/ssh-node-image:latest
        imagePullPolicy: Never
        ports:
        - name: ssh
          containerPort: 22
---
apiVersion: v1
kind: Service
metadata:
  name: ssh-node-%[2]d
  namespace: %[1]s
spec:
  selector:
    app: ssh-node-%[2]d
  ports:
  - name: ssh
    port: 22
    targetPort: 22
---
`, nodeNamespace, nodeId)
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
