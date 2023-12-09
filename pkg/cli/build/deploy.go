package build

import (
	"github.com/alecthomas/kingpin"
	"github.com/simple-container-com/welder/pkg/welder"
)

type Deploy struct {
	CommonParams
	BuildParams
	RunParams
	DeployParams
}

func (o *Deploy) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("deploy", "Run deployment steps specified for the modules")
	o.registerCommonFlags(cmd)
	o.registerBuildFlags(cmd)
	o.registerRunFlags(cmd)
	o.registerDeployFlags(cmd)
	cmd.Action(registerAction(o.Deploy))
	appVersion = a.Model().Version

	return cmd
}

func (o *Deploy) Deploy() error {
	buildCtx, err := o.ToBuildCtx("deploy", o.CommonParams)
	if err != nil {
		return err
	}
	if err := o.AddRunParams(buildCtx); err != nil {
		return err
	}
	deployCtx := welder.NewDeployContext(buildCtx, o.EnvNames)
	return deployCtx.Deploy()
}
