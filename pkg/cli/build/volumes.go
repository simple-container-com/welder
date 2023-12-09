package build

import (
	"github.com/alecthomas/kingpin"
	"github.com/fatih/color"
	"github.com/simple-container-com/welder/pkg/welder/runner"
)

type Volumes struct {
	CommonParams
	BuildParams
	RunParams

	DoWatch  bool
	Recreate bool
}

func (o *Volumes) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("volumes", "Create/update volumes with project root and all other volumes specified")
	o.registerCommonFlags(cmd)
	o.registerBuildFlags(cmd)
	o.registerRunFlags(cmd)
	appVersion = a.Model().Version

	cmd.Flag("watch", "Watch files for modifications and update volumes").
		Short('w').
		BoolVar(&o.DoWatch)

	cmd.Flag("recreate", "Recreate volumes from scratch").
		Short('r').
		BoolVar(&o.Recreate)

	cmd.Action(registerAction(o.Sync))
	return cmd
}

func (o *Volumes) Sync() error {
	buildCtx, err := o.ToBuildCtx("sync", o.CommonParams)
	if err != nil {
		return err
	}
	if err := o.AddRunParams(buildCtx); err != nil {
		return err
	}
	if o.DoWatch && !o.Parallel {
		color.Yellow("WARN: you're running watch without --parallel, this will sync single volume only!")
	}
	return buildCtx.VolumesSync(runner.SyncOpts{
		Recreate: o.Recreate,
		Watch:    o.DoWatch,
	})
}
