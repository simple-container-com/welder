package welder

import (
	"io"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

func TestRunBitbucketPipelinesPipe(t *testing.T) {
	RegisterTestingT(t)
	if os.Getenv("INSIDE_WELDER") == "true" {
		t.Skip("skipping test because it should only be run on host env")
	}

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/runs-bitbucket-pipes")
	defer cleanup()

	reader, stdout := io.Pipe()
	logger := util.NewStdoutLogger(stdout, stdout)
	eg := util.WaitForOutput(reader, func(output string) {
		Expect(output).To(ContainSubstring("Now we'll execute a pipe"))
		Expect(output).To(ContainSubstring("Hello from Docker!"))
		Expect(output).To(ContainSubstring("Executed pipe"))
	})
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &types.CommonCtx{}}, logger)
	buildCtx.SetRootDir(projectDir)
	Expect(buildCtx.Build()).To(BeNil())
	Expect(reader.Close()).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}
