package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"os"
	"testing"
)

func TestRunConditionalProfileWhenConditionIsTrue(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir := setupTempExampleProject(t, "testdata/conditional-profile")
	defer os.RemoveAll(projectDir)

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"another"}}}, logger)
	buildCtx.SetRootDir(projectDir)
	root, _ := ReadBuildRootDefinition(projectDir)
	Expect(buildCtx.IsProfileActive("some-profile", &root)).To(BeTrue())
}
