package welder

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestRunConditionalProfileWhenConditionIsTrue(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/conditional-profile")
	defer cleanup()

	logger := util.NewPrefixLogger("[build]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"another"}}}, logger)
	buildCtx.SetRootDir(projectDir)
	root, _ := ReadBuildRootDefinition(projectDir)
	Expect(buildCtx.IsProfileActive("some-profile", &root)).To(BeTrue())
}
