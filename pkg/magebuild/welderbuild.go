package magebuild

import (
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/config"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder"
	"github.com/smecsia/welder/pkg/welder/types"
	"os"
	"strings"
)

type WelderMageBuild struct {
	GoBuildContext     `yaml:",inline"`
	BuildViaAtlasBuild bool   `yaml:"-" default:"false" env:"BUILD_VIA_WELDER"`
	TestTarget         string `yaml:"-" default:"./..." env:"TEST_TARGET"`
	TestTags           string `yaml:"-" default:"osusergo" env:"TEST_TAGS"`
	atlasBuildCtx      *welder.BuildContext
}

func (mage *WelderMageBuild) Welder() *welder.BuildContext {
	return mage.atlasBuildCtx
}

func InitBuild() *WelderMageBuild {
	buildCtx := welder.NewBuildContext(&welder.BuildContext{
		CommonCtx: &types.CommonCtx{
			Verbose: os.Getenv("VERBOSE") == "true",
			BuildArgs: types.BuildArgs{
				"build-goals":      types.StringValue(os.Getenv("BUILD_GOALS")), // pass over the build goals to underneath build
				"build-via-welder": "false",                                     // make sure there's no recursion
			},
			Profiles: strings.Split(os.Getenv("PROFILES"), ","),
		}}, util.NewStdoutLogger(os.Stdout, os.Stderr))
	version, err := welder.NewVersionCtx(buildCtx, nil, nil)
	if err != nil {
		panic(errors.Wrap(err, "unable to init version context"))
	}
	fullVersion, err := version.Version()
	if err != nil {
		panic(errors.Wrap(err, "unable to calculate version"))
	}
	return config.Init("./build-targets.yaml", &WelderMageBuild{
		atlasBuildCtx: buildCtx,
		GoBuildContext: GoBuildContext{
			Version: fullVersion,
		}}, util.DefaultConsoleReader).(*WelderMageBuild)
}
