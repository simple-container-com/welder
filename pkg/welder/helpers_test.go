package welder

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestDockerVolumes(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")
	Expect(err).To(BeNil())

	buildCtx := BuildContext{CommonCtx: NewCommonContext(&CommonCtx{}, &util.NoopLogger{})}
	buildDef, module, err := buildCtx.ActualBuildDefinitionFor(&rootDef, "armory")
	Expect(err).To(BeNil())
	Expect(module.Name).To(Equal("armory"))

	settingsXmlPath, settingsSecurityXmlPath, _ := ensureMavenEnvExists()

	volumes, err := buildDef.Volumes.ToDockerVolumes(buildCtx.CommonCtx)
	Expect(err).To(BeNil())

	Expect(volumes).To(ContainElement(docker.Volume{
		Mode: "ro", HostPath: settingsXmlPath, ContPath: "/root/.m2/settings.xml",
	}))

	Expect(volumes).To(ContainElement(docker.Volume{
		Mode: "ro", HostPath: settingsSecurityXmlPath, ContPath: "/root/.m2/settings-security.xml",
	}))
}

func TestActualBuildOverridesDefaultEnvVariables(t *testing.T) {
	RegisterTestingT(t)
	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/default-env-override")
	defer cleanup()
	rootDef, err := ReadBuildRootDefinition(projectDir)

	Expect(err).To(BeNil())

	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"pipelines"}}}, &util.NoopLogger{})
	deployCtx := NewDeployContext(buildCtx, []string{"ddev"})
	deployCtx.SetRootDir(projectDir)
	deployDef, _, err := buildCtx.ActualDeployDefinitionFor(&rootDef, "test", deployCtx)

	Expect(err).To(BeNil())

	Expect(deployDef.Env).To(Equal(BuildEnv{
		"ENV_VALUE": "module-default",
		"VERSION":   "0.1.1-bpp",
	}))

	Expect(deployCtx.Deploy()).To(BeNil())
	outputFile := path.Join(projectDir, "output")
	_, err = os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	lines := strings.Split(string(outputFileBytes), "\n")
	Expect(lines[0]).To(Equal("module-default"))
	Expect(lines[1]).To(Equal("0.1.1-bpp"))
}
