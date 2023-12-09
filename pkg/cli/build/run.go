package build

import (
	"fmt"

	"github.com/alecthomas/kingpin"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type Run struct {
	BasicParams
	BuildParams
	RunParams
	Module  string
	Task    string
	StepIdx int
	Command string
}

func (o *Run) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("run", "Run a command")
	o.registerBasicFlags(cmd)
	o.registerBuildFlags(cmd)
	o.registerRunFlags(cmd)
	cmd.Flag("module", "Use this module's environment to run command").
		Short('m').
		StringVar(&o.Module)
	cmd.Flag("task", "Use this task's environment to run command").
		Short('t').
		StringVar(&o.Task)
	cmd.Flag("step", "Step index: use image specified in Nth step of the build").
		Short('s').
		IntVar(&o.StepIdx)
	cmd.Flag("command", "Command to start in container (if it's not action)").
		Short('c').
		StringVar(&o.Command)
	appVersion = a.Model().Version

	shellCmd := cmd.Command("shell", "Run command in shell")
	shellCmd.Arg("shellName", "Name of shell command to run").Default("sh").StringVar(&o.Command)
	shellCmd.Action(func(ctx *kingpin.ParseContext) error {
		return o.Shell(o.Command)
	})
	_, root, err := types.ReadBuildModuleDefinition("")
	if err != nil && o.Verbose {
		fmt.Println("WARN: Failed to read build definition: " + err.Error())
	} else {
		for k, v := range root.Tasks {
			task := v
			taskName := k
			taskCmd := cmd.Command(taskName, "Run task "+taskName+": "+task.Description)
			taskCmd.Action(func(ctx *kingpin.ParseContext) error {
				return o.Shell(taskName)
			})
		}
	}

	return cmd
}

func (o *Run) Shell(commandOrTask string) error {
	common := CommonParams{BasicParams: o.BasicParams}
	if o.Module != "" {
		common.Modules = []string{o.Module}
	}
	buildCtx, err := o.ToBuildCtx("run", common)
	if err != nil {
		return err
	}
	if err := o.AddRunParams(buildCtx); err != nil {
		return err
	}
	return buildCtx.Run(o.Task, o.StepIdx, commandOrTask)
}
