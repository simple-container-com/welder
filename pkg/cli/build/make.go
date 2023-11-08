package build

import (
	"github.com/alecthomas/kingpin"
)

type Make struct {
	CommonParams
	RunParams
	BuildParams
}

func (o *Make) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("make", "Run commands specified in build steps for each module")
	o.registerCommonFlags(cmd)
	o.registerBuildFlags(cmd)
	o.registerRunFlags(cmd)
	cmd.Action(registerAction(o.Make))
	appVersion = a.Model().Version

	return cmd
}

func (o *Make) Make() error {
	buildCtx, err := o.ToBuildCtx("make", o.CommonParams)
	if err != nil {
		return err
	}
	if err := o.AddRunParams(buildCtx); err != nil {
		return err
	}
	return buildCtx.Build()
}
