package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"os"
	"path"
	"strings"
	"testing"
)

func TestOverridePerStep(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/env-in-step-override")
	defer cleanup()

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Build()).To(BeNil())
	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	dirFile := path.Join(projectDir, "dir")
	_, err = os.Stat(dirFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+dirFile+" must exist")
	outputFileBytes, _ := os.ReadFile(outputFile)
	dirFileBytes, _ := os.ReadFile(dirFile)
	Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal("default-env=default\noverride-env=override"))
	Expect(strings.TrimSpace(string(dirFileBytes))).To(SatisfyAll(ContainSubstring(projectDir), ContainSubstring(path.Join(projectDir, "subdir"))))
}

func TestSyncWithMutagen(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/sync-with-mutagen")
	defer cleanup()

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{SyncMode: SyncModeExternal}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Run("test-task", 0, "test-task")).To(BeNil())
	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := os.ReadFile(outputFile)
	outputContent := strings.TrimSpace(string(outputFileBytes))
	createFile := path.Join(projectDir, "create-file")
	_, err = os.Stat(createFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+createFile+" must exist")
	createFileBytes, _ := os.ReadFile(createFile)
	createFileContent := strings.TrimSpace(string(createFileBytes))
	Expect(outputContent).To(ContainSubstring("git-ignore-content"))
	Expect(outputContent).To(ContainSubstring("ID=alpine"))
	Expect(outputContent).To(ContainSubstring("subdir"))
	Expect(createFileContent).To(ContainSubstring("create-file"))
}
