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
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LightJack05/ansible-operator/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage = "example.com/ansible-operator:v0.0.1"
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false
)

const (
	sshNodeImageName   = "localhost/ssh-node-image:latest"
	gitServerImageName = "localhost/git-server-image:latest"
	// gitServerNamespace must stay "default": the repos baked into the git
	// server image reference git-server.default.svc.cluster.local.
	gitServerNamespace = "default"
)

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
// The default setup requires Kind and CertManager.
// Specs are designed to be independent, so the suite can run in parallel via
// the ginkgo CLI (--procs); shared cluster fixtures are set up once below.
//
// To skip CertManager installation, set: CERT_MANAGER_INSTALL_SKIP=true
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting ansible-operator e2e test suite\n")
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.Timeout = 30 * time.Minute
	RunSpecs(t, "e2e suite", reporterConfig, suiteConfig)
}

// The first function runs on parallel process 1 only; all other processes
// block until it finishes, so the shared fixtures (images, git server,
// CertManager, CRDs and the controller-manager) exist exactly once before any
// spec runs.
var _ = SynchronizedBeforeSuite(func() []byte {
	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("building the ssh server image")
	cmd = exec.Command("make", "ssh-node-image", fmt.Sprintf("SSH_NODE_IMG=%s", sshNodeImageName))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build SSH node image")

	By("building the git server image")
	cmd = exec.Command("make", "git-server-image", fmt.Sprintf("GIT_SERVER_IMG=%s", gitServerImageName))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build git server image")

	// TODO(user): If you want to change the e2e test vendor from Kind,
	// ensure the image is built and available, then remove the following block.
	By("loading the images into Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	Expect(err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")
	err = utils.LoadImageToKindClusterWithName(sshNodeImageName)
	Expect(err).NotTo(HaveOccurred(), "Failed to load the ssh node image into Kind")
	err = utils.LoadImageToKindClusterWithName(gitServerImageName)
	Expect(err).NotTo(HaveOccurred(), "Failed to load the git server image into Kind")

	By("deploying the git server into the cluster")
	deployGitServer()

	setupCertManager()

	By("creating manager namespace")
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.Run(cmd)
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

	By("waiting for the controller-manager to become available")
	cmd = exec.Command("kubectl", "wait", "deployment",
		"-l", "control-plane=controller-manager",
		"-n", namespace,
		"--for=condition=Available",
		"--timeout=5m")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "controller-manager deployment did not become available")

	return nil
}, func(_ []byte) {})

// Teardown of the shared fixtures runs on process 1 only, after all other
// processes have finished their specs.
var _ = SynchronizedAfterSuite(func() {}, func() {
	By("undeploying the controller-manager")
	cmd := exec.Command("make", "undeploy")
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace)
	_, _ = utils.Run(cmd)

	By("removing the git server from the cluster")
	undeployGitServer()

	teardownCertManager()
})

// setupCertManager installs CertManager if needed for webhook tests.
// Skips installation if CERT_MANAGER_INSTALL_SKIP=true or if already present.
func setupCertManager() {
	if os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager installation (CERT_MANAGER_INSTALL_SKIP=true)\n")
		return
	}

	By("checking if CertManager is already installed")
	if utils.IsCertManagerCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "CertManager is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupCertManager = true

	By("installing CertManager")
	Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
}

// teardownCertManager uninstalls CertManager if it was installed by setupCertManager.
// This ensures we only remove what we installed.
func teardownCertManager() {
	if !shouldCleanupCertManager {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager cleanup (not installed by this suite)\n")
		return
	}

	By("uninstalling CertManager")
	utils.UninstallCertManager()
}
