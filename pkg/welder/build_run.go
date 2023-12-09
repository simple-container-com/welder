package welder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/pipelines"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/runner"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

// Run run command within environment defined by build config
func (buildCtx *BuildContext) Run(envTask string, envStepIdx int, commandOrTask string) error {
	if len(buildCtx.Modules) > 1 {
		return fmt.Errorf("could not run with more than 1 module specified, provided: %s", strings.Join(buildCtx.Modules, ","))
	}
	detectedModule, root, err := ReadBuildModuleDefinition(buildCtx.RootDir())
	if err != nil {
		return err
	}
	var moduleName string
	if detectedModule != nil {
		moduleName = detectedModule.Name
	}
	if len(buildCtx.Modules) == 1 {
		moduleName = buildCtx.Modules[0]
	}
	runID := root.ProjectNameOrDefault()
	_, taskExists := root.Tasks[commandOrTask]

	// provided task exists, running task and exiting early
	if taskExists {
		taskName := commandOrTask
		taskDefinition, err := buildCtx.ActualTaskDefinitionFor(&root, taskName, moduleName, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate task definition for task %s of module %s", taskName, moduleName)
		}
		runID = fmt.Sprintf("%s-%s", root.ProjectNameOrDefault(), taskName)
		buildCtx.ExecutingTask(taskName)
		defer buildCtx.ExecutedTask(taskName)
		err = buildCtx.RunScripts(taskName, runID, &root, moduleName, taskDefinition.ToRunSpec(taskName))
		if err != nil {
			return err
		}
		return nil
	}

	// otherwise, trying to run it as a command (if task/module context is defined)
	if moduleName == "" && envTask == "" {
		return fmt.Errorf("could not determine task/module to use for running")
	}

	var runConfig RunSpec
	if moduleName != "" {
		// use module's build environment
		if len(root.Modules) > 1 {
			runID = fmt.Sprintf("%s-%s", root.ProjectNameOrDefault(), moduleName)
		}
		buildRunCtx, err := buildCtx.calcModuleBuildRunContext(&root, moduleName, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate build run context")
		}
		steps := buildRunCtx.buildDef.Steps
		if envStepIdx > len(steps)-1 {
			return fmt.Errorf("step does not exist: %d for module %s", envStepIdx, moduleName)
		}
		currentStepsDef := steps[envStepIdx]
		currentStepsDef, err = root.ActualStepsDefinitionFor(&buildRunCtx.buildDef, &currentStepsDef)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate effective steps definition for module %s and step %d", moduleName, envStepIdx)
		}
		if currentStepsDef.Task != "" && envTask == "" {
			envTask = currentStepsDef.Task
		} else {
			runConfig = currentStepsDef.Step.ToRunSpec(runID, buildRunCtx.buildDef.CommonRunDefinition)
		}
	}
	if envTask != "" {
		// use task's environment
		runID = fmt.Sprintf("%s-%s", root.ProjectNameOrDefault(), envTask)
		taskDefinition, err := buildCtx.ActualTaskDefinitionFor(&root, envTask, moduleName, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate task definition for task %s of module %s", envTask, moduleName)
		}
		runConfig = taskDefinition.ToRunSpec(runID)
		buildCtx.ExecutingTask(envTask)
		defer buildCtx.ExecutedTask(envTask)
	}

	// build custom image if specified
	if runConfig.CustomImage.IsValid() {
		if runConfig.CustomImage.Name == "" {
			runConfig.CustomImage.Name = runConfig.Name
		}
		tags, err := buildCtx.buildDockerImage(&root, moduleName, dockerBuildParams{
			dockerImage: runConfig.CustomImage,
			allowReuse:  !buildCtx.NoCache,
			allowLabels: true,
		})
		if err != nil {
			return err
		}
		if len(tags) == 0 {
			return errors.Errorf("image build didn't return any tags for an image")
		}
		runConfig.Image = tags[0]
	}

	run := runner.NewRun(buildCtx.CommonCtx)
	containerParams, err := run.CalcRunInContainerParams(root.ProjectNameOrDefault(), root.ConfiguredRootPath(), runConfig.RunCfg, &root.Default.MainVolume)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate run in container params")
	}
	dockerRun, err := docker.NewRun(runID, runConfig.Image)
	if err != nil {
		return errors.Wrapf(err, "failed to init container run")
	}
	if err := run.ConfigureVolumes(dockerRun, containerParams); err != nil {
		return errors.Wrapf(err, "failed to configure volumes")
	}
	runCtx := docker.RunContext{
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Stdin:     os.Stdin,
		User:      buildCtx.Username,
		WorkDir:   containerParams.WorkDir,
		Debug:     buildCtx.Verbose,
		CurrentOS: buildCtx.OS(),
		Silent:    !buildCtx.Verbose,
		Tty:       true,
		Env:       runConfig.RunCfg.Env.ToBuildEnv(runConfig.RunCfg.InjectEnvRegex(buildCtx.CommonCtx)...),
		Detached:  false,
		Logger:    buildCtx.Logger(),
	}

	return dockerRun.
		SetReuseContainers(buildCtx.ReuseContainers).
		SetDisableCache(buildCtx.NoCache).
		SetCleanupOrphans(buildCtx.RemoveOrphans).
		SetContext(buildCtx.GoContext()).
		MountDockerSocket().
		KeepEnvironmentWithEachCommand().
		Run(runCtx, commandOrTask)
}

func (buildCtx *BuildContext) RunScriptsOfSimpleStep(name string, rawStep RunAfterStepDefinition, root *RootBuildDefinition, module string) error {
	runID := root.ProjectNameOrDefault()
	if len(root.Modules) > 1 {
		runID = fmt.Sprintf("%s-%s", root.ProjectNameOrDefault(), module)
	}

	buildRunCtx, err := buildCtx.calcModuleBuildRunContext(root, module, nil)
	if err != nil {
		return err
	}
	var run *RunSpec
	step, err := ActualSimpleStepsDefinitionFor(root, buildRunCtx.buildCtx, buildRunCtx.module, &rawStep.SimpleStepDefinition)
	if err != nil {
		return err
	}
	if len(step.Scripts) > 0 {
		convRun := step.ToRunSpec(name, buildRunCtx.buildDef.CommonRunDefinition)
		run = &convRun
	}
	if run == nil {
		buildCtx.Logger().Debugf("Skip running step %s: script is empty")
		return nil
	}
	if err := buildRunCtx.buildCtx.RunScripts(name, fmt.Sprintf("%s-%s", runID, name), root, module, *run); err != nil {
		return err
	}
	return nil
}

func (buildCtx *BuildContext) RunScripts(action string, runID string, root *RootBuildDefinition, moduleName string, runSpec RunSpec) error {
	subCtx := NewBuildContext(buildCtx, buildCtx.Logger().SubLogger(runSpec.Name))

	if runSpec.RunIf != "" {
		subCtx.Logger().Debugf("Checking condition to run: %q", runSpec.RunIf)
		running, err := CheckRunCondition(root, *buildCtx, moduleName, runSpec)
		if err != nil {
			return errors.Wrapf(err, "failed to verify run condition for %q", runSpec.RunIf)
		}
		if !running {
			subCtx.Logger().Logf(" - Skip execution of %s due to condition %q", action, runSpec.RunIf)
			return nil
		}
	}

	stepBuildStartedAt := time.Now()
	defer func() {
		buildCtx.SetLastExecOutput(subCtx.LastExecOutput())
		subCtx.Logger().Logf(" - Finished %s in %s", action, util.FormatDuration(time.Since(stepBuildStartedAt)))
	}()

	// if runOn is not set to "host", we should run it in container
	run := runner.NewRun(subCtx.CommonCtx)
	runInContainerParams, err := run.CalcRunInContainerParams(root.ProjectNameOrDefault(), root.ConfiguredRootPath(), runSpec.RunCfg, &root.Default.MainVolume)
	if err != nil {
		return errors.Wrapf(err, "failed to calc run in container params")
	}
	if runSpec.RunOn.IsContainer() && !buildCtx.ForceOnHost {
		if runSpec.CustomImage.IsValid() {
			return subCtx.runInCustomImageContainer(action, runID, root, moduleName, runSpec)
		} else if runSpec.Image != "" {
			return run.RunInContainer(action, runID, runInContainerParams, runSpec)
		}
		return errors.Errorf("image or customImage must be specified for %q", action)
	}
	// otherwise - run it on host
	return run.RunOnHost(action, runID, runInContainerParams, runSpec)
}

func (buildCtx *BuildContext) RunSteps(action string, root *RootBuildDefinition, module string, deployCtx *DeployContext) error {
	runID := root.ProjectNameOrDefault()
	if len(root.Modules) > 1 {
		runID = fmt.Sprintf("%s-%s", root.ProjectNameOrDefault(), module)
	}
	moduleBuildStartedAt := time.Now()

	buildCtx.Logger().Logf(" - %s module '%s'...", strings.Title(action), module)

	buildRunCtx, err := buildCtx.calcModuleBuildRunContext(root, module, deployCtx)
	if err != nil {
		return err
	}
	subCtx := buildRunCtx.buildCtx

	defer func() {
		buildCtx.Logger().Logf(" - Finished %s module %s in %s", action, module, util.FormatDuration(time.Since(moduleBuildStartedAt)))
	}()

	// Run each step separately
	for stepIdx, rawStep := range buildRunCtx.buildDef.Steps {
		var run *RunSpec
		stepRunID := runID

		step, err := root.ActualStepsDefinitionFor(&buildRunCtx.buildDef, &rawStep)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate effective step definition for step %s of module %s", stepRunID, module)
		}
		stepName := strconv.Itoa(stepIdx)
		if step.Name != "" {
			stepRunID = fmt.Sprintf("%s-%s", runID, step.Name)
			stepName = step.Name
		} else if step.Task != "" {
			stepRunID = fmt.Sprintf("%s-%s", runID, step.Task)
			stepName = step.Task
		} else if step.Pipe != "" {
			stepRunID = fmt.Sprintf("%s-%s", runID, step.Pipe)
			stepName = step.Pipe
		} else {
			stepRunID = fmt.Sprintf("%s-%d", runID, stepIdx)
		}

		if len(step.Step.Scripts) > 0 {
			convRun := step.Step.ToRunSpec(stepName, step.ToRunDefinition(buildRunCtx.buildDef.CommonRunDefinition))
			run = &convRun
		} else if step.Task != "" {
			action, err := subCtx.ActualTaskDefinitionFor(root, step.Task, module, deployCtx)
			if err != nil {
				return errors.Wrapf(err, "failed to calcualate task definition for task %s of module %s", step.Task, module)
			}
			convRun := action.ToRunSpec(step.Task)
			subCtx.ExecutingTask(step.Task)
			defer subCtx.ExecutedTask(step.Task)
			run = &convRun
		} else if step.Pipe != "" {
			if buildDef, _, err := subCtx.ActualBuildDefinitionFor(root, module); err != nil {
				return errors.Wrapf(err, "failed to calcualate build definition for pipe %s of module %s", step.Pipe, module)
			} else if err := subCtx.runBitbucketPipe(runID, step.Pipe, step.Env, root, buildDef); err != nil {
				return errors.Wrapf(err, "failed to invoke bitbucket pipe %s of module %s", step.Pipe, module)
			}
			continue
		}
		if run == nil {
			return errors.Errorf("neither of [step, task, pipe] were specified for %s of step %s of module %s", action, stepName, module)
		}
		if err := subCtx.RunScripts(fmt.Sprintf("%s step %q of module %s", action, stepName, module), stepRunID, root, module, *run); err != nil {
			return err
		}
	}
	return nil
}

func (buildCtx *BuildContext) runBitbucketPipe(runID string, pipeName string, pipeEnv BuildEnv, root *RootBuildDefinition, buildDef BuildDefinition) error {
	pipe := pipelines.NewPipe(pipeName,
		pipelines.NewBitbucketContext(buildCtx.CommonCtx).
			WithProjectRoot(root.ConfiguredRootPath()).
			WithProjectName(root.ProjectNameOrDefault()),
	)
	if !buildCtx.IsRunningInCI() {
		if token, err := pipelines.GenerateBitbucketOauthToken(false); err == nil {
			pipe.OAuthToken = token
		} else {
			buildCtx.Logger().Debugf("WARN: Failed to generate bitbucket oauth token: %s", err)
		}
	}
	image, err := pipe.DockerImage()
	if err != nil {
		return errors.Wrapf(err, "failed to calculate docker image for pipe %s", pipeName)
	}
	env, err := pipe.PipelinesEnv()
	if err != nil {
		return errors.Wrapf(err, "failed to calculate pipelines env for pipe %s", pipeName)
	}
	mergedEnv := ParseBuildEnv(env)
	MergeMapIfEmpty(pipeEnv, mergedEnv)
	run := runner.NewRun(buildCtx.CommonCtx)
	runCfg := CommonRunDefinition{CommonSimpleRunDefinition: CommonSimpleRunDefinition{
		Env: mergedEnv, Volumes: buildDef.Volumes,
	}}
	spec := RunSpec{
		Name:   pipeName,
		Image:  image,
		RunCfg: runCfg,
	}
	runParams, err := run.CalcRunInContainerParams(root.ProjectNameOrDefault(), root.ConfiguredRootPath(), runCfg, &root.Default.MainVolume)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate run params")
	}
	return run.RunInContainer(pipeName, runID, runParams, spec)
}
