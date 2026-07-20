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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LightJack05/ansible-operator/test/utils"
)

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
		"",
		privateKeySecretName,
		hostKeySecretName,
		port,
		ignoreHostKeys,
	))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsibleHost")
}

func createValidAnsibleHostWithVars(name, namespace, hostname, username, privateKeySecretName, hostKeySecretName, vars string, port int, ignoreHostKeys bool) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateAnsibleHost(
		name,
		namespace,
		hostname,
		username,
		vars,
		privateKeySecretName,
		hostKeySecretName,
		port,
		ignoreHostKeys,
	))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsibleHost")
}
func templateAnsibleHost(name, namespace, hostname, username, hostVars, privateKeySecretName, hostKeySecretName string, port int, ignoreHostKeys bool) string {
	return fmt.Sprintf(
		`
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleHost
metadata:
  name: %[1]s 
  namespace: %[3]s 
spec:
  ansibleName: %[1]s
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
  hostVars: %[9]s
---
`,
		name, hostname, namespace, username, privateKeySecretName, hostKeySecretName, port, ignoreHostKeys, hostVars)
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
	createSSHHosts(sshNodesNamespace)
}

func createSSHHosts(namespace string) {
	var manifests strings.Builder
	for i := range sshNodeCount {
		manifests.WriteString(templateSSHServer(namespace, i))
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
			"--namespace", namespace,
			"--for=condition=Available",
			"--timeout=5m",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "SSH deployment did not become available")
	}
}

// catFileOnSSHNode returns the contents of a file inside the given SSH node
// by exec'ing cat in its pod. Only stdout is returned, so kubectl warnings on
// stderr cannot end up in the file contents.
func catFileOnSSHNode(namespace string, nodeId int, filePath string) string {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "exec", fmt.Sprintf("deployments/ssh-node-%d", nodeId),
		"-n", namespace, "--", "cat", filePath)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	Expect(err).NotTo(HaveOccurred(),
		"Failed to cat %q on ssh-node-%d: %s", filePath, nodeId, stderr.String())
	return stdout.String()
}

// fileExistsOnSSHNode reports whether a file exists inside the given SSH node
// by exec'ing stat in its pod. Any failure other than stat reporting a missing
// file (e.g. kubectl cannot reach the pod) fails the test, since that is not
// the same as the file being absent.
func fileExistsOnSSHNode(namespace string, nodeId int, filePath string) bool {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "exec", fmt.Sprintf("deployments/ssh-node-%d", nodeId),
		"-n", namespace, "--", "stat", filePath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true
	}
	Expect(stderr.String()).To(ContainSubstring("No such file or directory"),
		"Failed to check for %q on ssh-node-%d: %s", filePath, nodeId, stderr.String())
	return false
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

// templateGitServer renders a Deployment and Service for the git server image,
// which serves the baked-in test repositories over HTTP and SSH.
func templateGitServer(namespace string) string {
	return fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: git-server
  namespace: %[1]s
spec:
  selector:
    matchLabels:
      app: git-server
  template:
    metadata:
      labels:
        app: git-server
    spec:
      containers:
      - name: git-server
        image: %[2]s
        imagePullPolicy: Never
        ports:
        - name: ssh
          containerPort: 22
        - name: http
          containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: git-server
  namespace: %[1]s
spec:
  selector:
    app: git-server
  ports:
  - name: ssh
    port: 22
    targetPort: 22
  - name: http
    port: 80
    targetPort: 80
---
`, namespace, gitServerImageName)
}

// deployGitServer deploys the git server serving the baked-in test
// repositories and waits for it to become available. It lives in the default
// namespace for the whole suite, so playbooks can be fetched from
// git-server.default.svc.cluster.local during tests.
func deployGitServer() {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateGitServer(gitServerNamespace))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create git server resources")

	cmd := exec.Command("kubectl", "wait", "deployments/git-server",
		"--namespace", gitServerNamespace,
		"--for=condition=Available",
		"--timeout=5m",
	)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Git server deployment did not become available")
}

// undeployGitServer removes the git server resources created by
// deployGitServer. Errors are ignored so suite teardown proceeds even if the
// resources are already gone.
func undeployGitServer() {
	cmd := exec.Command("kubectl", "delete", "-f", "-", "--ignore-not-found=true")
	cmd.Stdin = strings.NewReader(templateGitServer(gitServerNamespace))
	_, _ = utils.Run(cmd)
}

func createSSHKeySecret(namespace, secretName string) {
	GinkgoHelper()
	privateKey := generateSSHPrivateKey()
	applySSHKeySecret(namespace, secretName, privateKey)
}

func applySSHKeySecret(namespace, secretName, privateKey string) {
	GinkgoHelper()
	secretManifest := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
  ssh_key: %s
`, secretName, namespace, base64.StdEncoding.EncodeToString([]byte(privateKey)))

	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(secretManifest)
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create SSH key secret")
}

func generateSSHPrivateKey() string {
	GinkgoHelper()
	// Generate a new SSH private key using the ssh-keygen command
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-b", "2048", "-f", "/tmp/temp_ssh_key", "-N", "")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to generate SSH private key")

	// Read the generated private key from the file
	privateKeyBytes, err := os.ReadFile("/tmp/temp_ssh_key")
	Expect(err).NotTo(HaveOccurred(), "Failed to read generated SSH private key")

	// Clean up the temporary key file
	err = os.Remove("/tmp/temp_ssh_key")
	Expect(err).NotTo(HaveOccurred(), "Failed to remove temporary SSH key file")

	return string(privateKeyBytes)
}

func createAnsibleGroup(name, namespace, groupVars string, hostNames, groupnames []string) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateAnsibleGroup(name, namespace, groupVars, hostNames, groupnames))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsibleGroup")
}

func templateAnsibleGroup(name, namespace, groupVars string, hostNames, groupNames []string) string {
	type AnsibleGroupTemplateData struct {
		Name       string
		Namespace  string
		GroupVars  string
		HostNames  []string
		GroupNames []string
	}

	tmpl, err := template.New("ansibleGroup").Parse(`
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleGroup
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  ansibleName: {{ .Name }}
  groupVars: "{{ .GroupVars }}"
  hosts:{{ if not .HostNames }} []{{ end }}
{{- range .HostNames }}
  - name: {{ . }}
{{- end }}
  groups:{{ if not .GroupNames }} []{{ end }}
{{- range .GroupNames }}
  - name: {{ . }}
{{- end }}
`)
	Expect(err).NotTo(HaveOccurred(), "Failed to parse AnsibleGroup template")
	ansibleGroup := AnsibleGroupTemplateData{
		Name:       name,
		Namespace:  namespace,
		GroupVars:  groupVars,
		HostNames:  hostNames,
		GroupNames: groupNames,
	}
	var renderedGroup strings.Builder
	err = tmpl.Execute(&renderedGroup, ansibleGroup)
	Expect(err).NotTo(HaveOccurred(), "Failed to render AnsibleGroup template")
	output := renderedGroup.String()
	fmt.Println(output)
	return output
}

func createInlineAnsiblePlaybook(name, namespace, playbook, requirements string) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateInlineAnsiblePlaybook(name, namespace, playbook, requirements))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsiblePlaybook")
}

func templateInlineAnsiblePlaybook(name, namespace, playbook, requirements string) string {
	manifest := fmt.Sprintf(
		`
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsiblePlaybook
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  inline:
    playbook: |
%[3]s
`,
		name, namespace, indentLines(playbook, 6))
	if requirements != "" {
		manifest += fmt.Sprintf("    requirements: |\n%s\n", indentLines(requirements, 6))
	}
	return manifest
}

func deleteAnsiblePlaybook(name, namespace string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "delete", "ansibleplaybook", name, "-n", namespace)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete AnsiblePlaybook")
}

func createGitAnsiblePlaybook(name, namespace, repoURL, ref, playbookPath, requirementsPath string) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateGitAnsiblePlaybook(name, namespace, repoURL, ref, playbookPath, requirementsPath))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsiblePlaybook")
}

func templateGitAnsiblePlaybook(name, namespace, repoURL, ref, playbookPath, requirementsPath string) string {
	manifest := fmt.Sprintf(
		`
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsiblePlaybook
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  git:
    repo:
      url: %[3]s
`,
		name, namespace, repoURL)
	if ref != "" {
		manifest += fmt.Sprintf("      ref: %s\n", ref)
	}
	manifest += fmt.Sprintf("    playbookPath: %s\n", playbookPath)
	if requirementsPath != "" {
		manifest += fmt.Sprintf("    requirementsPath: %s\n", requirementsPath)
	}
	return manifest
}

// indentLines prefixes every non-empty line with n spaces so multi-line
// content can be embedded in a YAML block scalar.
func indentLines(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

func createAnsibleReconcileJob(name, namespace, schedule, playbookName string) {
	GinkgoHelper()
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(templateAnsibleReconcileJob(name, namespace, schedule, playbookName))
	_, err := utils.Run(applyCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create AnsibleReconcileJob")
}

func deleteAnsibleReconcileJob(name, namespace string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "delete", "ansiblereconcilejob", name, "-n", namespace)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete AnsibleReconcileJob")
}

func templateAnsibleReconcileJob(name, namespace, schedule, playbookName string) string {
	return fmt.Sprintf(
		`
---
apiVersion: ansible-operator.lightjack.de/v1alpha1
kind: AnsibleReconcileJob
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  schedule: "%[3]s"
  playbookRef:
    name: %[4]s
---
`,
		name, namespace, schedule, playbookName)
}
