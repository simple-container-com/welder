package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/git/mock"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestModuleVersionBumpPatch(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/example")
	defer os.RemoveAll(tmpProjectDir)

	versionCtx := readVersionContext(tmpProjectDir, "third")

	version, err := versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("1.0.0-beta"))

	err = versionCtx.BumpPatch()
	Expect(err).To(BeNil())

	versionCtx = readVersionContext(tmpProjectDir, "third")
	version, err = versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("1.0.0"))
}

func TestModuleVersionBumpMinor(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/example")
	defer os.RemoveAll(tmpProjectDir)

	versionCtx := readVersionContext(tmpProjectDir, "armory")

	version, err := versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("0.0.2-1234567890f-test"))

	err = versionCtx.BumpMinor()
	Expect(err).To(BeNil())

	versionCtx = readVersionContext(tmpProjectDir, "armory")
	version, err = versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("0.1.0-1234567890f-test"))
}

func TestRootVersionBumpMajor(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/example")
	defer os.RemoveAll(tmpProjectDir)

	versionCtx := readVersionContext(tmpProjectDir)

	version, err := versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("0.0.1-1234567-${project:module.name}"))

	err = versionCtx.BumpMajor()
	Expect(err).To(BeNil())

	versionCtx = readVersionContext(tmpProjectDir)
	version, err = versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("1.0.0-1234567-${project:module.name}"))
}

func TestVersionFromArgs(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/version-from-args")
	defer os.RemoveAll(tmpProjectDir)

	rootDef, err := ReadBuildRootDefinition(tmpProjectDir)
	Expect(err).To(BeNil())

	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"pipelines"}}},
		util.NewStdoutLogger(os.Stdout, os.Stderr))
	versionCtx, err := NewVersionCtx(buildCtx, &rootDef, nil)
	Expect(err).To(BeNil())

	os.Setenv("_BITBUCKET_BUILD_NUMBER", "100")
	os.Setenv("_bamboo_buildNumber", "50")
	version, err := versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("0.5.100"))

	buildCtx = NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"bamboo"}}},
		util.NewStdoutLogger(os.Stdout, os.Stderr))
	versionCtx, err = NewVersionCtx(buildCtx, &rootDef, nil)
	Expect(err).To(BeNil())

	version, err = versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("0.5.50"))
}

func TestTaskOutputCapture(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/task-output-capture")
	defer os.RemoveAll(tmpProjectDir)

	rootDef, err := ReadBuildRootDefinition(tmpProjectDir)
	Expect(err).To(BeNil())

	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, util.NewStdoutLogger(os.Stdout, os.Stderr))
	buildCtx.SetRootDir(tmpProjectDir)
	versionCtx, err := NewVersionCtx(buildCtx, &rootDef, nil)
	Expect(err).To(BeNil())
	version, err := versionCtx.Version()
	Expect(err).To(BeNil())
	Expect(version).To(Equal("1.0"))

	Expect(buildCtx.Run("", 0, "test-capture-output")).To(BeNil())
	outputFile := path.Join(tmpProjectDir, "output")
	_, err = os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal("" +
		"Output from alpine: Linux"))

	err = buildCtx.Run("", 0, "recursion")
	outputFileBytes, _ = ioutil.ReadFile(outputFile)
	Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal("output of recursion is \"true\""))
	Expect(err).To(BeNil())
}

func TestResolveEnvInVersion(t *testing.T) {
	RegisterTestingT(t)
	tmpProjectDir := createTempExampleProject(t, "testdata/version-from-args")
	defer os.RemoveAll(tmpProjectDir)

	rootDef, err := ReadBuildRootDefinition(tmpProjectDir)
	Expect(err).To(BeNil())

	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, util.NewStdoutLogger(os.Stdout, os.Stderr))
	deployCtx := NewDeployContext(buildCtx, []string{"ddev"})
	depDef, _, err := buildCtx.ActualDeployDefinitionFor(&rootDef, "test", deployCtx)
	Expect(err).To(BeNil())
	Expect(depDef.Steps[0].Step.Scripts[0]).To(Equal("echo ddev-0.5.0"))

}

func readVersionContext(tmpProjectDir string, modules ...string) *VersionCtx {
	rootDef, err := ReadBuildRootDefinition(tmpProjectDir)
	Expect(err).To(BeNil())

	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Modules: modules}}, util.NewStdoutLogger(os.Stdout, os.Stderr))
	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f-test", nil)
	gitMock.On("Branch").Return("feature/test", nil)
	buildCtx.SetGitClient(&gitMock)
	versionCtx, err := NewVersionCtx(buildCtx, &rootDef, nil)
	Expect(err).To(BeNil())
	return versionCtx
}
