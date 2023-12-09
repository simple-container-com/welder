package build

import (
	"github.com/alecthomas/kingpin"
)

type All struct {
	BuildParams
	Username string
}

func (o *All) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("all", "Build the project and ")
	cmd.Flag("username", "Run builds under this username").
		Short('u').
		StringVar(&o.Username)

	o.registerBuildFlags(cmd)
	appVersion = a.Model().Version
	cmd.Action(registerAction(o.All))
	return cmd
}

func (o *All) All() error {
	makeCmd := Make{
		RunParams: RunParams{Username: o.Username},
	}

	err := makeCmd.Make()
	if err != nil {
		return err
	}

	dockerCmd := Docker{
		BuildParams: o.BuildParams,
	}

	err = dockerCmd.Build()

	if err != nil {
		return err
	}

	return dockerCmd.Push()
}
