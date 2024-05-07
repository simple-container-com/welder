package magebuild

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/config"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type WelderMageBuild struct {
	GoBuildContext `yaml:",inline"`
	BuildViaWelder bool   `yaml:"-" default:"false" env:"BUILD_VIA_WELDER"`
	TestTarget     string `yaml:"-" default:"./..." env:"TEST_TARGET"`
	TestTags       string `yaml:"-" default:"osusergo" env:"TEST_TAGS"`
	welderBuildCtx *welder.BuildContext
}

func (mage *WelderMageBuild) Welder() *welder.BuildContext {
	return mage.welderBuildCtx
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
		},
	}, util.NewStdoutLogger(os.Stdout, os.Stderr))
	version, err := welder.NewVersionCtx(buildCtx, nil, nil)
	if err != nil {
		panic(errors.Wrap(err, "unable to init version context"))
	}
	fullVersion, err := version.Version()
	if err != nil {
		panic(errors.Wrap(err, "unable to calculate version"))
	}
	return config.Init("./build-targets.yaml", &WelderMageBuild{
		welderBuildCtx: buildCtx,
		GoBuildContext: GoBuildContext{
			Version: fullVersion,
		},
	}, util.DefaultConsoleReader).(*WelderMageBuild)
}
