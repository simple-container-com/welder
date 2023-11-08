package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestRunCommandAfterDockerBuild(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir := setupTempExampleProject(t, "testdata/after-docker-build")
	defer os.RemoveAll(projectDir)

	buildLogger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, buildLogger)
	buildCtx.SetRootDir(projectDir)
	Expect(buildCtx.BuildDocker([]string{})).To(BeNil())

	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	Expect(strings.Split(string(outputFileBytes), "\n")).To(ContainElements(
		"build image=first",
		"build tags[0].image=docker.simple-container.com/deng/test/after-docker-build-first",
		"build tags[0].tag=latest",
		"build image=second",
		"build tags[0].tag=latest",
		"build tags[0].image=docker.simple-container.com/deng/test/after-docker-build",
		"build tags[0].digest=",
		"build tags[1].tag=latest-extra-tag",
		"build tags[1].image=docker.simple-container.com/deng/test/after-docker-build-extra",
		"build tags[1].digest=",
	))

	pushLogger := util.NewPrefixLogger("[push]", false)
	pushCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, pushLogger)
	pushCtx.SetRootDir(projectDir)
	Expect(pushCtx.PushDocker([]string{})).To(BeNil())

	outputFileBytes, _ = ioutil.ReadFile(outputFile)
	Expect(strings.Split(string(outputFileBytes), "\n")).To(ContainElements(
		"push image=second",
		"push tags[0].tag=latest",
		"push tags[0].image=docker.simple-container.com/deng/test/after-docker-build",
		MatchRegexp(`push tags\[0\]\.digest=sha256:[a-f0-9]+`),
		"push tags[1].tag=latest-extra-tag",
		"push tags[1].image=docker.simple-container.com/deng/test/after-docker-build-extra",
		MatchRegexp(`push tags\[1\].digest=sha256:[a-f0-9]+`),
	))

	def, err := ReadOutDockerDefinition(path.Join(projectDir, BuildOutputDir, OutDockerFileName))
	Expect(err).To(BeNil())

	Expect(def.Modules).To(HaveLen(1))
	Expect(def.Modules[0].DockerImages).To(HaveLen(2))

}
