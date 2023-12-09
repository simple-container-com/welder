package welder

import (
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestRunCommandAfterDockerBuild(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/after-docker-build")
	defer cleanup()

	buildLogger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, buildLogger)
	buildCtx.SetRootDir(projectDir)
	Expect(buildCtx.BuildDocker([]string{})).To(BeNil())

	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := os.ReadFile(outputFile)
	Expect(strings.Split(string(outputFileBytes), "\n")).To(ContainElements(
		// first
		"build image=first",
		"build tags[0].image=docker.simple-container.com/deng/test/after-docker-build-first",
		"build tags[0].tag=latest",
		// second
		"build image=second",
		"build tags[0].tag=latest",
		"build tags[0].image=docker.simple-container.com/deng/test/after-docker-build",
		"build tags[0].digest=",
		"build tags[1].tag=latest-extra-tag",
		"build tags[1].image=docker.simple-container.com/deng/test/after-docker-build-extra",
		"build tags[1].digest=",
		// third
		"task-build image=third",
		"task-build tags[0].tag=third1",
		"task-build tags[0].image=docker.simple-container.com/deng/test/after-docker-build-task",
		"task-build tags[0].digest=",
		"task-build tags[1].tag=third2",
		"task-build tags[1].image=docker.simple-container.com/deng/test/after-docker-build-task",
		"task-build tags[1].digest=",
	))

	pushLogger := util.NewPrefixLogger("[push]", false)
	pushCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, pushLogger)
	pushCtx.SetRootDir(projectDir)
	Expect(pushCtx.PushDocker([]string{})).To(BeNil())

	outputFileBytes, _ = os.ReadFile(outputFile)
	Expect(strings.Split(string(outputFileBytes), "\n")).To(ContainElements(
		// second
		"push image=second",
		"push tags[0].tag=latest",
		"push tags[0].image=docker.simple-container.com/deng/test/after-docker-build",
		MatchRegexp(`push tags\[0\]\.digest=sha256:[a-f0-9]+`),
		"push tags[1].tag=latest-extra-tag",
		"push tags[1].image=docker.simple-container.com/deng/test/after-docker-build-extra",
		MatchRegexp(`push tags\[1\].digest=sha256:[a-f0-9]+`),
		// third
		"task-push image=third",
		"task-push tags[0].tag=third1",
		"task-push tags[0].image=docker.simple-container.com/deng/test/after-docker-build-task",
		MatchRegexp(`task-push tags\[0\]\.digest=sha256:[a-f0-9]+`),
		"task-push tags[1].tag=third2",
		"task-push tags[1].image=docker.simple-container.com/deng/test/after-docker-build-task",
		MatchRegexp(`task-push tags\[1\].digest=sha256:[a-f0-9]+`),
	))

	def, err := ReadOutDockerDefinition(path.Join(projectDir, BuildOutputDir, OutDockerFileName))
	Expect(err).To(BeNil())

	Expect(def.Modules).To(HaveLen(1))
	Expect(def.Modules[0].DockerImages).To(HaveLen(3))
}
