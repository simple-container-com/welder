package docker

import (
	"context"
	. "github.com/onsi/gomega"
	"os"
	"testing"
	"time"
)

func TestDockerExecCopyAndRemoveContainer(t *testing.T) {
	RegisterTestingT(t)

	du, err := NewDefaultUtil(context.Background())
	Expect(err).To(BeNil())

	run, err := NewRun("some-test-run", "ubuntu")
	Expect(err).To(BeNil())

	err = run.Run(RunContext{Detached: true, Debug: true, Stdout: os.Stdout, Stderr: os.Stderr})
	Expect(err).To(BeNil())

	etcHostsCat, err := du.ExecInContainer(run.ContainerID(), "cat /etc/hosts")
	Expect(err).To(BeNil())
	Expect(etcHostsCat).NotTo(BeEmpty())

	etcHostsCopy, err := du.ReadFileFromContainer(run.ContainerID(), "/etc/hosts")
	Expect(err).To(BeNil())
	Expect(etcHostsCopy).NotTo(BeEmpty())

	Expect(etcHostsCat).To(Equal(etcHostsCopy))

	err = du.ForceRemoveContainer(run.ContainerID(), 2*time.Second)
	Expect(err).To(BeNil())

	exists, err := du.GetContainerStatus(run.ContainerID())
	Expect(err).To(BeNil())
	Expect(exists.Running).To(BeFalse())
	Expect(exists.Exists).To(BeFalse())
}

func TestDetectOSDistribution(t *testing.T) {
	RegisterTestingT(t)
	if os.Getenv("BITBUCKET_BUILD_NUMBER") != "" {
		t.Skipf("Skipping test because it is running in CI build")
	}
	du, err := NewDefaultUtil(context.Background())
	Expect(err).To(BeNil())

	Expect(du.DetectOSDistributionFromImage("hashicorp/http-echo").Name()).To(Equal(OSDistributionUnknown))
	Expect(du.DetectOSDistributionFromImage("alpine:latest").Name()).To(Equal(OSDistributionAlpine))
	Expect(du.DetectOSDistributionFromImage("docker:latest").Name()).To(Equal(OSDistributionAlpine))
	Expect(du.DetectOSDistributionFromImage("ubuntu:latest").Name()).To(Equal(OSDistributionUbuntu))
}

func TestCleanupDockerID(t *testing.T) {
	RegisterTestingT(t)
	Expect(CleanupDockerID("projectName-Some step name")).To(Equal("projectNameSomestepnam"))
	Expect(CleanupDockerID("atlassian/git-secrets-scan:0.6.1")).To(Equal("atlassiangitsecretsscan0"))
}
