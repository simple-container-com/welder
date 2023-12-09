package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/pipelines"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type BitbucketPipelines struct {
	curDir    string
	pipelines *pipelines.BitbucketPipelines
	Params    pipelines.BitbucketPipelinesRunParams
}

func (o *BitbucketPipelines) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("bitbucket-pipelines", "Interact with bitbucket-pipelines")

	curDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	o.curDir, err = filepath.Abs(curDir)
	if err != nil {
		panic(err)
	}

	o.registerGenerateCommand(cmd)
	o.registerExecCommand(cmd)

	return cmd
}

func (o *BitbucketPipelines) registerExecCommand(cmd *kingpin.CmdClause) {
	execute := cmd.Command("execute", "Execute Bitbucket Pipelines for the project")
	ctx := types.NewCommonContext(&types.CommonCtx{Verbose: true}, util.NewPrefixLogger("", false))
	ctx.InitGitClientIfNeeded()
	ctx.CancelOnSignal()

	execute.Flag("skip-pipes", "Skip pipes execution when running Bitbucket Pipeline").
		BoolVar(&o.Params.SkipPipes)
	execute.Flag("atlassian", "Enable Atlassian internal tweaks (default)").
		Default("true").
		BoolVar(&o.Params.AtlassianMode)

	bbpFile := filepath.Join(o.curDir, "bitbucket-pipelines.yml")
	if pp, err := pipelines.NewBitbucketPipelines(bbpFile, ctx); err == nil {
		o.pipelines = pp
		execute.Flag("parallel", "Enable parallel execution (false by default)").
			BoolVar(&o.pipelines.Parallel)
		execute.Flag("debug", "Enable debug output").
			BoolVar(&o.pipelines.Verbose)
		execute.Flag("branch", "Override current branch name").
			StringVar(&o.pipelines.Branch)
		execute.Flag("max-parallel", "Limit max parallel threads to run steps in parallel").
			IntVar(&o.pipelines.ParallelCount)
		allCmd := execute.Command("all", "Run all steps")
		allCmd.Action(func(ctx *kingpin.ParseContext) error {
			return o.Execute(&o.Params)
		})
		addedSteps := make(map[string]bool)
		for _, step := range pp.Config().AllSteps() {
			if step.Step.Name != nil && !addedSteps[*step.Step.Name] {
				stepName := *step.Step.Name
				subCmdName := strings.ToLower(strings.ReplaceAll(stepName, " ", "-"))
				stepCmd := execute.Command(subCmdName, fmt.Sprintf("Run step '%s'", stepName))
				stepCmd.Action(func(ctx *kingpin.ParseContext) error {
					o.Params.StepName = stepName
					return o.Execute(&o.Params)
				})
				addedSteps[stepName] = true
			}
		}
	} else {
		execute.Action(registerAction(o.NotAPipelinesProject))
	}
}

func (o *BitbucketPipelines) registerGenerateCommand(cmd *kingpin.CmdClause) {
	bbp := welder.BitbucketPipelines{}
	generate := cmd.Command("generate", "Generate BBP for the project")

	generate.Flag("root", "Path to the root of the project (default: current directory)").
		Short('r').
		Default(o.curDir).
		StringVar(&bbp.RootPath)

	generate.Flag("main-branch", "Name of the main branch (default: master)").
		Short('b').
		Default("master").
		StringVar(&bbp.MainBranch)
	generate.Action(registerAction(bbp.Generate))
}

func (o *BitbucketPipelines) NotAPipelinesProject() error {
	return errors.Errorf("%s is not a Bitbucket Pipelines project (no bitbucket-pipelines.yml found)", o.curDir)
}

func (o *BitbucketPipelines) Execute(params *pipelines.BitbucketPipelinesRunParams) error {
	jwtToken := os.Getenv("PIPELINES_JWT_TOKEN")
	if jwtToken != "" {
		o.pipelines.JWTToken = jwtToken
	}
	if token, err := pipelines.GenerateBitbucketOauthToken(false); err != nil {
		if _, err := pipelines.GenerateBitbucketOauthToken(true); err == nil {
			if token, err := pipelines.GenerateBitbucketOauthToken(false); err != nil {
				o.pipelines.OAuthToken = token
			}
		}
	} else {
		o.pipelines.OAuthToken = token
	}
	return o.pipelines.Run(params)
}
