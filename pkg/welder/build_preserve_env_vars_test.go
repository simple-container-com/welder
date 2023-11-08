package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestBuildPreservesEnvVariables(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir := setupTempExampleProject(t, "testdata/preserve-env-variables")
	defer os.RemoveAll(projectDir)

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &types.CommonCtx{}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Build()).
		To(BeNil())

	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	output := strings.TrimSpace(string(outputFileBytes))
	Expect(output).To(ContainSubstring("SAME_SCRIPT=value"))
	Expect(output).To(ContainSubstring("OTHER_SCRIPT=no-value"))
}
