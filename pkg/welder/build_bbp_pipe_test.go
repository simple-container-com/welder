package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"io"
	"os"
	"testing"
)

func TestRunBitbucketPipelinesPipe(t *testing.T) {
	RegisterTestingT(t)
	if os.Getenv("INSIDE_WELDER") == "true" {
		t.Skip("skipping test because it should only be run on host env")
	}

	_, projectDir := setupTempExampleProject(t, "testdata/runs-bitbucket-pipes")
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(projectDir)

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
