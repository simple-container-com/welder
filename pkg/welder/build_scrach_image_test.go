package welder

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

func TestBuildWithScratchImage(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/use-scratch-images")
	defer cleanup()

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &types.CommonCtx{Verbose: true}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Run("collect-output-of-echo-image", 0, "collect-output-of-echo-image")).
		To(BeNil())

	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	Expect(strings.TrimSpace(string(outputFileBytes))).To(ContainSubstring("Hello from Docker!This message shows that your installation appears to be working correctly."))
}
