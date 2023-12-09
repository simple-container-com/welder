package welder

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestBuildWithCustomImage(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name            string
		module          string
		expectedContent string
	}{
		{
			name:   "run build with custom dockerfiles",
			module: "custom-image-steps",
			expectedContent: "" +
				"module=custom-image-steps; user=root; project=custom-image\n" +
				"module=custom-image-steps; user=root",
		},
		{
			name:   "run build with custom dockerfiles",
			module: "custom-image-with-task",
			expectedContent: "" +
				"module=custom-image-with-task; user=root; project=custom-image",
		},
	}
	for _, testCase := range testCases {
		tc := testCase // for proper closures
		t.Run(tc.name, func(t *testing.T) {
			_, projectDir, cleanup := setupTempExampleProject(t, "testdata/custom-image")
			defer cleanup()

			logger := util.NewPrefixLogger("[build]", false)
			buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{NoCache: true, Modules: []string{tc.module}}}, logger)
			buildCtx.SetRootDir(projectDir)

			Expect(buildCtx.Build()).To(BeNil())

			outputFile := path.Join(projectDir, "output")
			_, err := os.Stat(outputFile)
			Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
			outputFileBytes, _ := ioutil.ReadFile(outputFile)
			Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal(tc.expectedContent))

			logger = util.NewPrefixLogger("[docker]", false)
			dockerCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, logger)
			dockerCtx.SetRootDir(projectDir)
			Expect(dockerCtx.BuildDocker([]string{})).To(BeNil())
		})
	}
}
