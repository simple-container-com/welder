package build

import (
	"github.com/alecthomas/kingpin"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder"
	"github.com/smecsia/welder/pkg/welder/types"
	"os/user"
	"strings"
)

var appVersion string

type BasicParams struct {
	appVersion      string
	Sox             bool
	SkipTests       bool
	Bamboo          bool
	Verbose         bool
	NotStrict       bool
	NoCache         bool
	ReuseContainers bool
	RemoveOrphans   bool
	ForceOnHost     bool
	SyncMode        string
	PrintTimestamps bool
}

type CommonParams struct {
	BasicParams
	Parallel      bool
	ParallelCount int
	Modules       []string
}

type BuildParams struct {
	BasicParams
	Args       map[string]string
	Profiles   []string
	SimulateOS string
}

type RunParams struct {
	Username string
}

type DeployParams struct {
	EnvNames []string
}

func (o *DeployParams) registerDeployFlags(cmd *kingpin.CmdClause) {
	cmd.Flag("env", "Environment to use with Micros Service").
		Short('e').
		Required().
		StringsVar(&o.EnvNames)
}
func (o *RunParams) registerRunFlags(cmd *kingpin.CmdClause) {
	cmd.Flag("username", "Run builds under this username").
		Short('u').
		StringVar(&o.Username)
}

func (o *BasicParams) registerBasicFlags(cmd *kingpin.CmdClause) {
	cmd.Flag("verbose", "Print more detailed messages about process").
		Short('v').
		BoolVar(&o.Verbose)
	cmd.Flag("timestamps", "Prefix build output with current date/time").
		Short('T').
		BoolVar(&o.PrintTimestamps)
	cmd.Flag("disable-strict", "Disable strict mode").
		Short('Z').
		BoolVar(&o.NotStrict)
	cmd.Flag("sox", "Activate SOX compliant mode").
		Short('S').
		BoolVar(&o.Sox)
	cmd.Flag("skip-tests", "Activate no-tests mode").
		Short('B').
		BoolVar(&o.SkipTests)
	cmd.Flag("bamboo", "Activate 'Bamboo' mode (intended for use from Bamboo only: DEPRECATED)").
		Hidden().
		BoolVar(&o.Bamboo)
	cmd.Flag("no-cache", "Disable cache when building Docker images").
		Short('N').
		BoolVar(&o.NoCache)
	cmd.Flag("volume-mode", "Specifies volume mode (bind|copy|external|add|volume)").
		Short('M').
		StringVar(&o.SyncMode)
	cmd.Flag("reuse-containers", "Do not remove containers after creation, try to reuse them").
		Short('R').
		BoolVar(&o.ReuseContainers)
	cmd.Flag("remove-orphans", "Remove orphan containers (if found) with the same runID").
		Short('D').
		BoolVar(&o.RemoveOrphans)
	cmd.Flag("on-host", "Run all commands on host environment instead of Docker").
		Short('H').
		BoolVar(&o.ForceOnHost)
}

func (o *CommonParams) registerCommonFlags(cmd *kingpin.CmdClause) {
	o.registerBasicFlags(cmd)
	_, rootDef, _ := types.ReadBuildModuleDefinition("")
	cmd.Flag("module", "The modules to build ("+strings.Join(rootDef.ModuleNames(), "|")+")").
		Short('m').
		EnumsVar(&o.Modules, rootDef.ModuleNames()...)
	cmd.Flag("parallel", "Allow concurrent builds").
		Short('P').
		BoolVar(&o.Parallel)
	cmd.Flag("parallel-count", "Max number of parallel builds (0 for unlimited)").
		Short('c').
		IntVar(&o.ParallelCount)
}

func (o *BuildParams) registerBuildFlags(cmd *kingpin.CmdClause) {
	o.Args = make(map[string]string)
	_, rootDef, _ := types.ReadBuildModuleDefinition("")
	cmd.Flag("arg", "Build argument").
		Short('a').
		StringMapVar(&o.Args)
	cmd.Flag("profile", "Build profiles to activate ("+strings.Join(rootDef.ProfileNames(), "|")+")").
		Short('p').
		EnumsVar(&o.Profiles, rootDef.ProfileNames()...)
	cmd.Flag("os", "Simulate host operating system").
		Short('O').
		StringVar(&o.SimulateOS)
}

func (o *BuildParams) ToBuildCtx(ctxName string, common CommonParams) (*welder.BuildContext, error) {
	args := make(types.BuildArgs)
	for k, v := range o.Args {
		args[k] = types.StringValue(v)
	}
	ctx := &welder.BuildContext{
		CommonCtx: &types.CommonCtx{
			Profiles:         o.Profiles,
			BuildArgs:        args,
			Verbose:          common.Verbose,
			Strict:           !common.NotStrict,
			SoxEnabled:       common.Sox,
			SkipTestsEnabled: common.SkipTests,
			Parallel:         common.Parallel,
			ParallelCount:    common.ParallelCount,
			Modules:          common.Modules,
			SimulateOS:       o.SimulateOS,
			NoCache:          common.NoCache,
			SyncMode:         types.SyncMode(common.SyncMode),
			ReuseContainers:  common.ReuseContainers,
			RemoveOrphans:    common.RemoveOrphans,
			ForceOnHost:      common.ForceOnHost,
		},
	}
	logger := util.NewPrefixLogger(ctxName, ctx.Verbose)
	if common.PrintTimestamps {
		logger = util.NewTimestampPrefixLogger(ctxName, ctx.Verbose)
	}
	context := welder.NewBuildContext(ctx, logger)
	context.SetVersion(appVersion)
	return context, nil
}

func (o *RunParams) AddRunParams(ctx *welder.BuildContext) error {
	// If username wasn't intentionally specified, trying to use current username of the host
	if o.Username == "" {
		curUser, err := user.Current()
		if err != nil {
			return err
		}
		ctx.Username = curUser.Username
	}
	return nil
}
