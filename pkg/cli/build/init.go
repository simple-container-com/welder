package build

import (
	"github.com/alecthomas/kingpin"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder"
)

type Init struct {
	NonInteractive bool
	Preset         string
	Simple         bool
}

type ActionFunction func() error

func (o *Init) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("init", "Initialize current project for building a service")
	cmd.Flag("non-interactive", "Non-interactive mode").
		Short('B').
		BoolVar(&o.NonInteractive)
	cmd.Flag("preset", "Enable preset for project type (e.g. maven)").
		Short('P').
		StringVar(&o.Preset)
	cmd.Flag("simple", "Simple mode (ask only most important questions)").
		Short('S').
		BoolVar(&o.Simple)
	cmd.Action(registerAction(o.Init))
	appVersion = a.Model().Version
	return cmd
}

func (o *Init) Init() error {
	console := util.NewDefaultConsole()
	if o.NonInteractive {
		console.AlwaysRespondDefault()
	}
	if init, err := welder.NewInit(console, o.Preset); err != nil {
		return err
	} else {
		init.Simple = o.Simple
		return init.RunWizard()
	}
}

func registerAction(f ActionFunction) kingpin.Action {
	return func(ctx *kingpin.ParseContext) error {
		return f()
	}
}
