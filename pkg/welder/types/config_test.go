package types_test

import (
	"fmt"
	. "github.com/onsi/gomega"
	. "github.com/smecsia/welder/pkg/welder/types"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestReadOutDockerDefinition(t *testing.T) {
	RegisterTestingT(t)

	def, err := ReadOutDockerDefinition(path.Join("testdata", ".welder-out-docker.yaml"))
	Expect(err).To(BeNil())

	Expect(def.Modules[0].Name).To(Equal("armory"))
	Expect(def.Modules[1].Name).To(Equal("trebuchet"))
	Expect(def.Modules[0].DockerImages[0].Digests[0].Image).To(Equal("docker.simple-container.com/test/deng/sox/armory"))
	Expect(def.Modules[0].DockerImages[0].Digests[0].Digest).To(Equal("sha256:36d04b6c4164191d1b18445fc3282be2e8b89062168d3556db7da591df2858bb"))
	Expect(def.Modules[1].Name).To(Equal("trebuchet"))
	Expect(def.Modules[0].DockerImages[0].Name).To(Equal("service"))
	Expect(def.Modules[0].DockerImages[0].Digests[0].Tag).To(Equal("7473ba1"))
	Expect(def.Modules[0].DockerImages[0].Digests[1].Tag).To(Equal("docker.simple-container.com/test/deng/sox/armory:7473ba18330d8cca50705fe1dc8f3b9393d99e61"))

	Expect(def.Modules[1].DockerImages[0].Name).To(Equal("service"))
	Expect(def.Modules[1].DockerImages[0].Digests[0].Tag).To(Equal("36502d5"))
	Expect(def.Modules[1].DockerImages[0].Digests[1].Tag).To(Equal("docker.simple-container.com/test/deng/sox/trebuchet:36502d5b068f0cad6f468a47bc4c2b5fd7003c26"))

}

func TestDetectBuildContextFromModule(t *testing.T) {
	RegisterTestingT(t)

	cwd, err := os.Getwd()
	Expect(err).To(BeNil())
	armoryDir := filepath.Join(cwd, "testdata", "services", "armory")
	examplePath, err := filepath.Abs(filepath.Join(armoryDir, "..", ".."))
	Expect(err).To(BeNil())

	rootDir, subDir, err := DetectBuildContext(armoryDir)
	Expect(err).To(BeNil())
	Expect(rootDir).To(Equal(examplePath))
	Expect(subDir).To(Equal(path.Join("services", "armory")))

	module, rootDef, err := ReadBuildModuleDefinition(armoryDir)
	Expect(err).To(BeNil())
	Expect(module).NotTo(BeNil())

	Expect(module.Name).To(Equal("armory"))
	Expect(rootDef.ProjectName).To(Equal("deployinator"))
}

func TestDetectBuildContextFromOutsideOfProject(t *testing.T) {
	RegisterTestingT(t)

	cwd, err := os.Getwd()
	Expect(err).To(BeNil())

	projectDir := filepath.Join(cwd, "..", "..", "..", "..")

	moduleCfg, _, err := ReadBuildModuleDefinition(projectDir)
	Expect(moduleCfg).To(BeNil())
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(MatchRegexp("could not determine project root"))
}

func TestDetectBuildContextFromRoot(t *testing.T) {
	RegisterTestingT(t)

	cwd, err := os.Getwd()
	Expect(err).To(BeNil())

	exampleDir := filepath.Join(cwd, "testdata")

	rootDir, subDir, err := DetectBuildContext(exampleDir)
	Expect(err).To(BeNil())
	Expect(rootDir).To(Equal(exampleDir))
	Expect(subDir).To(Equal("."))

	module, rootDef, err := ReadBuildModuleDefinition(exampleDir)
	Expect(err).To(BeNil())
	Expect(module).To(BeNil())

	Expect(rootDef.ProjectName).To(Equal("deployinator"))
}

func TestBuildRootSpec(t *testing.T) {
	RegisterTestingT(t)

	sut, err := ReadBuildRootDefinition("testdata")
	Expect(err).To(BeNil())
	assert.NoError(t, err)
	Expect(sut.ProjectName).To(Equal("deployinator"))
	Expect(sut.ProjectRoot).To(Equal("services"))
	Expect(sut.Default.Build.Env["SOME_OTHER_VAR"]).To(Equal(StringValue("value")))
	Expect(sut.Profiles["skip-tests"].Build.Env["BUILD_ARGS"]).To(Equal(StringValue("${env:BUILD_ARGS} -DskipTests")))
	Expect(sut.Profiles["sox"].Activation.Sox).To(Equal(true))
	Expect(len(sut.Modules)).To(Equal(4))
	Expect(sut.Modules[0].Name).To(Equal("armory"))
	Expect(sut.Modules[0].Build.ContainerWorkDir).To(Equal("/some/directory"))
	Expect(sut.Modules[0].Path).To(Equal("services/armory"))
	Expect(sut.Modules[0].Build.Steps[0].Step.Image).To(Equal("maven:${arg:maven-version}"))
	Expect(sut.Modules[0].DockerImages[0].Name).To(Equal("service"))
	Expect(sut.Modules[0].DockerImages[0].DockerFile).To(Equal("./Dockerfile"))
	Expect(sut.RootDirPath()).To(Equal("testdata"))
}

func TestBuildRootSpecForMoreRecentVersion(t *testing.T) {
	RegisterTestingT(t)

	_, err := ReadBuildRootDefinition("testdata/more-recent-welder-version")
	Expect(err).NotTo(BeNil())
	Expect(err.Error()).To(ContainSubstring("more recent version of welder"))
	Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("welder (99.0.0), current version: %s", RootBuildDefinitionSchemaVersion)))
}
