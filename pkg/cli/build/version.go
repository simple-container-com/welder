package build

import (
	"fmt"
	"github.com/alecthomas/kingpin"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/welder"
)

type Version struct {
	CommonParams
	BuildParams
	Value string
}

func (o *Version) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("version", "Operate on version of the project")
	o.registerCommonFlags(cmd)
	o.registerBuildFlags(cmd)
	printCmd := cmd.Command("print", "Print version from build config")
	printCmd.Action(registerAction(o.Print))
	bumpPatch := cmd.Command("bump-patch", "Bump patch-version")
	bumpPatch.Action(registerAction(o.BumpPatch))
	bumpMinor := cmd.Command("bump-minor", "Bump minor-version")
	bumpMinor.Action(registerAction(o.BumpMinor))
	bumpMajor := cmd.Command("bump-major", "Bump major-version")
	bumpMajor.Action(registerAction(o.BumpMajor))
	setVersion := cmd.Command("set", "Bump major-version")
	setVersion.Action(registerAction(o.Set))
	setVersion.Arg("version", "Version value to set").StringVar(&o.Value)
	appVersion = a.Model().Version

	return cmd
}

func (o *Version) BumpMinor() error {
	return o.ctx().BumpMinor()
}

func (o *Version) BumpMajor() error {
	return o.ctx().BumpMajor()
}

func (o *Version) BumpPatch() error {
	return o.ctx().BumpPatch()
}

func (o *Version) Set() error {
	return o.ctx().SetVersionInConfig(o.Value)
}

func (o *Version) Print() error {
	val, err := o.ctx().Version()
	fmt.Println(val)
	return err
}

func (o *Version) ctx() *welder.VersionCtx {
	buildCtx, err := o.ToBuildCtx("version", o.CommonParams)
	if err != nil {
		panic(errors.Wrapf(err, "failed to build context"))
	}
	verCtx, err := welder.NewVersionCtx(buildCtx, nil, nil)
	if err != nil {
		panic(errors.Wrapf(err, "failed to create version context"))
	}
	return verCtx
}
