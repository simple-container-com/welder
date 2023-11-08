package pipelines

import (
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/otiai10/copy"
	"github.com/smecsia/welder/pkg/git"
	"github.com/smecsia/welder/pkg/git/mock"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestReadSimplePipelinesFile(t *testing.T) {
	RegisterTestingT(t)

	pipelines, err := ReadBitbucketPipelinesSchemaFile("testdata/simple/bitbucket-pipelines.yml")

	Expect(err).To(BeNil())

	Expect(pipelines.Pipelines.Default).To(HaveLen(1))
	step, err := pipelines.Pipelines.Default.ToStep(0)
	Expect(err).To(BeNil())
	Expect(*step.Step.Name).To(Equal("Print Hello"))
	Expect(step.Step.Script).To(HaveLen(3))
	script, err := step.Step.GetScript(0)
	Expect(err).To(BeNil())
	Expect(script).To(Equal("echo hello > output"))
	Expect(step.Step.Script.IsScript(1)).To(BeTrue())
	Expect(step.Step.Script.IsPipe(2)).To(BeTrue())
}

func TestReadPipelinesSchemaFileWithParallel(t *testing.T) {
	RegisterTestingT(t)

	pipelines, err := ReadBitbucketPipelinesSchemaFile("testdata/with-parallel/bitbucket-pipelines.yml")

	Expect(err).To(BeNil())

	Expect(pipelines.Pipelines.Default).To(HaveLen(1))
	Expect(pipelines.Pipelines.Default.IsParallel(0)).To(BeTrue())
	parallel, err := pipelines.Pipelines.Default.ToParallel(0)
	Expect(err).To(BeNil())
	Expect(parallel.Parallel).To(HaveLen(2))
	Expect(*parallel.Parallel[0].Step.Name).To(Equal("parallel1"))
	Expect(*parallel.Parallel[1].Step.Name).To(Equal("parallel2"))
}

func TestReadPipelinesFileWithDefinitions(t *testing.T) {
	RegisterTestingT(t)

	pipelines, err := ReadBitbucketPipelinesSchemaFile("testdata/with-definitions/bitbucket-pipelines.yml")

	Expect(err).To(BeNil())

	Expect(pipelines.Pipelines.Default).To(HaveLen(3))
	Expect(pipelines.Pipelines.Default.IsParallel(0)).To(BeTrue())
	parallel, err := pipelines.Pipelines.Default.ToParallel(0)
	Expect(err).To(BeNil())
	Expect(parallel.Parallel).To(HaveLen(8))
	Expect(*parallel.Parallel[0].Step.Name).To(Equal("Build random-wait-plugin"))
	Expect(parallel.Parallel[0].Step.Script).To(HaveLen(3))
	Expect(parallel.Parallel[0].Step.Script.IsPipe(0)).To(BeTrue())
	Expect(parallel.Parallel[0].Step.Script.IsScript(2)).To(BeTrue())
	pipe, err := parallel.Parallel[0].Step.Script.GetPipe(0)
	Expect(err).To(BeNil())
	Expect(pipe.Pipe).To(Equal("atlassian/artifactory-sidekick:v1"))
	script, err := parallel.Parallel[0].Step.Script.GetScript(2)
	Expect(err).To(BeNil())
	Expect(script).To(Equal("welder make --timestamps -m random-wait-plugin"))
	Expect(*parallel.Parallel[7].Step.Name).To(Equal("Build poco-plugin"))
	Expect(pipelines.Pipelines.Default.IsStep(1)).To(BeTrue())
	step1, err := pipelines.Pipelines.Default.ToStep(1)
	Expect(err).To(BeNil())
	Expect(*step1.Step.Name).To(Equal("Validate BBP config was regenerated"))
	Expect(pipelines.Pipelines.Branches).To(HaveLen(1))
	branch, ok := pipelines.Pipelines.Branches.Get("main")
	Expect(ok).To(BeNil())
	Expect(branch).To(HaveLen(3))

	step2, err := pipelines.Pipelines.Default.ToStep(2)
	Expect(err).To(BeNil())
	Expect(*step2.Step.Name).To(Equal("Done"))
	image1, err := step1.Step.GetImage(pipelines)
	Expect(err).To(BeNil())
	Expect(image1).To(Equal("docker-proxy.services.atlassian.com/sox/deng/welder-pbc:latest"))
	image2, err := step2.Step.GetImage(pipelines)
	Expect(err).To(BeNil())
	Expect(image2).To(Equal("alpine"))
}

func TestRunDefaultPipeline(t *testing.T) {
	RegisterTestingT(t)
	pipelines, dir, callback := preparePipelinesProject(t, "with-branch", "feature/test")
	defer callback()
	Expect(pipelines.Run(nil)).To(BeNil())
	outputFile := readOutputFile(t, dir)
	Expect(outputFile).To(ContainSubstring("HELLO default"))
	Expect(outputFile).To(ContainSubstring("ID=ubuntu"))
}

func TestRunPipelineForBranch(t *testing.T) {
	RegisterTestingT(t)
	pipelines, dir, callback := preparePipelinesProject(t, "with-branch", "master")
	defer callback()
	Expect(pipelines.Run(nil)).To(BeNil())

	outputFile := readOutputFile(t, dir)
	Expect(outputFile).To(ContainSubstring("HELLO master"))
	Expect(outputFile).To(ContainSubstring("ID=alpine"))
}

func readOutputFile(t *testing.T, dir string) string {
	RegisterTestingT(t)
	outputFile := path.Join(dir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	return string(outputFileBytes)
}

func preparePipelinesProject(t *testing.T, exampleName string, branch string) (*BitbucketPipelines, string, func()) {
	dir := createTempPipelinesProject(t, fmt.Sprintf("testdata/%s", exampleName))
	ctx := types.NewCommonContext(&types.CommonCtx{Verbose: true}, util.NewPrefixLogger(fmt.Sprintf("[%s]", exampleName), false))
	injectGitMock(ctx, branch)
	pipelines, err := NewBitbucketPipelines(filepath.Join(dir, "bitbucket-pipelines.yml"), ctx)
	Expect(err).To(BeNil())
	return pipelines, dir, func() {
		_ = os.RemoveAll(dir)
	}
}

func createTempPipelinesProject(t *testing.T, pathToExample string) string {
	RegisterTestingT(t)
	depDir, err := ioutil.TempDir(os.TempDir(), "bbp")
	Expect(err).To(BeNil())
	err = copy.Copy(pathToExample, depDir)
	Expect(err).To(BeNil())
	return depDir
}

func injectGitMock(ctx *types.CommonCtx, branch string) {
	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f", nil)
	gitMock.On("Branch").Return(branch, nil)
	gitMock.On("Remotes").Return([]git.Remote{{
		Name: "origin",
		URLs: []string{"git@bitbucket.org:atlassianlabs/welder.git"},
	}}, nil)
	ctx.SetGitClient(&gitMock)
}
