package pipelines

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/pipelines/schema"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/types"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

type BitbucketPipelines struct {
	*BitbucketContext
}

// NewBitbucketPipelines initializes a new Pipelines object
func NewBitbucketPipelines(bbpFile string, commonCtx *types.CommonCtx) (*BitbucketPipelines, error) {
	config, err := ReadBitbucketPipelinesSchemaFile(bbpFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", bbpFile)
	}

	projectRoot, err := filepath.Abs(filepath.Dir(bbpFile))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get absolute path for %s", bbpFile)
	}
	projectName := filepath.Base(projectRoot)

	if commonCtx == nil {
		commonCtx = &types.CommonCtx{}
	}
	commonCtx.SetRootDir(projectRoot)
	return &BitbucketPipelines{
		BitbucketContext: NewBitbucketContext(commonCtx).
			WithProjectRoot(projectRoot).
			WithProjectName(projectName).
			WithConfig(config).
			WithConfigFile(bbpFile),
	}, nil
}

// ReadBitbucketPipelinesSchemaFile initializes a new Pipelines object
func ReadBitbucketPipelinesSchemaFile(bbpFile string) (schema.BitbucketPipelinesSchemaJson, error) {
	res := schema.BitbucketPipelinesSchemaJson{}
	fileBytes, err := os.ReadFile(bbpFile)
	if err != nil {
		return res, errors.Wrapf(err, "failed to read %s", bbpFile)
	}

	err = yaml.Unmarshal(fileBytes, &res)
	if err != nil {
		return res, errors.Wrapf(err, "failed to unmarshal %s", bbpFile)
	}
	return res, nil
}

func (pipelines *BitbucketPipelines) Run(runParams *BitbucketPipelinesRunParams) error {
	pipelines.Logger().Logf("Detecting current git branch...")

	var branch string
	var err error
	if runParams != nil && pipelines.Branch != "" {
		branch = pipelines.Branch
	} else {
		branch, err = pipelines.GitClient().Branch()
		if err != nil {
			return errors.Wrapf(err, "failed to get current branch")
		}
	}
	pipelines.Logger().Logf("Current git branch: %s", branch)
	var steps schema.StepsOrParallel
	p := pipelines.config.Pipelines
	if p.Branches.Contains(branch) {
		pipelines.Logger().Logf("Pipelines configuration for branch %s found", branch)
		steps, err = p.Branches.Get(branch)
		if err != nil {
			return errors.Wrapf(err, "failed to get steps for branch %q", branch)
		}
	} else {
		pipelines.Logger().Logf("Pipelines configuration for branch %s not found, using default", branch)
		steps = p.Default
	}

	if runParams != nil {
		steps = runParams.filterSteps(steps)
	}
	for idx := range steps {
		if steps.IsParallel(idx) {
			parallel, err := steps.ToParallel(idx)
			if err != nil {
				return errors.Wrapf(err, "failed to convert step %d to parallel", idx)
			}
			err = pipelines.runParallel(parallel, runParams)
			if err != nil {
				return errors.Wrapf(err, "failed to run parallel %d", idx)
			}
		} else {
			step, err := steps.ToStep(idx)
			if err != nil {
				return errors.Wrapf(err, "failed to convert step %d to step", idx)
			}
			err = pipelines.runStep(step, runParams)
			if err != nil {
				return errors.Wrapf(err, "failed to run step %d", idx)
			}
		}
	}

	return nil
}

func (pipelines *BitbucketPipelines) runStep(step schema.Step, runParams *BitbucketPipelinesRunParams) error {
	stepStartedAt := time.Now()
	stepName := "step"
	if step.Step.Name != nil {
		stepName = *step.Step.Name
	}
	pipelines.Logger().Logf("=== Starting '%s' ===", stepName)
	defer func() {
		pipelines.Logger().Logf("=== Finished '%s' in %s ===", stepName, util.FormatDuration(time.Since(stepStartedAt)))
	}()
	runID := docker.CleanupDockerID(fmt.Sprintf("%s-%s", pipelines.projectName, stepName))
	subCtx := types.NewCommonContext(pipelines.CommonCtx, pipelines.Logger().SubLogger(stepName))
	env, err := pipelines.PipelinesEnv()
	if err != nil {
		return errors.Wrapf(err, "failed to obtain pipelines env variables")
	}
	subCtx.Logger().Logf("Environment variables:\n%s", strings.Join(env, "\n"))

	runCfg := types.CommonRunDefinition{
		CommonSimpleRunDefinition: types.CommonSimpleRunDefinition{
			Env:     types.ParseBuildEnv(env),
			Volumes: nil,
			WorkDir: pipelines.projectRoot,
		},
	}
	image, err := step.Step.GetImage(pipelines.config)
	if err != nil {
		return errors.Wrapf(err, "failed to get Docker image name for step %s", stepName)
	}
	if runParams != nil {
		image = runParams.NormalizeDockerReference(image)
	}

	imageRef, err := docker.ResolveDockerImageReference(image)
	if err != nil {
		return errors.Wrapf(err, "failed to get resolve image reference from %s for step %s", image, stepName)
	}

	subCtx.Logger().Logf("Environment variables:\n%s", strings.Join(env, "\n"))
	subCtx.Logger().Logf("Images used:\n\tbuild: %s", imageRef.Reference)

	dockerRun, err := pipelines.NewDockerRun(runID, stepName, imageRef.Reference)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize docker run for step %s", stepName)
	}

	var eg errgroup.Group
	logReader, stdout := io.Pipe()
	logReaderErr, stderr := io.Pipe()
	eg.Go(util.ReaderToLogFunc(logReader, false, "", subCtx.Logger(), fmt.Sprintf("%s with image %s", stepName, imageRef.Reference)))
	eg.Go(util.ReaderToLogFunc(logReaderErr, true, "ERR: ", subCtx.Logger(), fmt.Sprintf("%s with image %s", stepName, imageRef.Reference)))

	runCtx := docker.RunContext{
		Stdout:          stdout,
		Stderr:          stderr,
		WorkDir:         pipelines.RootDir(),
		Debug:           pipelines.Verbose,
		ErrorOnExitCode: true,
		Detached:        true,
		Env:             runCfg.Env.ToBuildEnv(),
		Logger:          pipelines.Logger(),
	}
	dockerRun.
		SetEnv(runCfg.Env.ToBuildEnv()...).
		SetContext(pipelines.GoContext()).
		SetReuseContainers(pipelines.ReuseContainers).
		SetDisableCache(pipelines.NoCache).
		SetCleanupOrphans(pipelines.RemoveOrphans).
		KeepEnvironmentWithEachCommand()

	if util.SliceContains(step.Step.Services, "docker") {
		dockerRun.MountDockerSocket()
	}

	_, err = dockerRun.PrepareContainer(runCtx)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize docker container for step %s", stepName)
	}

	defer func(dockerRun *docker.Run) {
		pipelines.Logger().Logf("Removing docker container %s", dockerRun.ContainerID())
		_ = dockerRun.Destroy()
	}(dockerRun)

	scripts := step.Step.Script
	for idx := range scripts {
		if runParams != nil && runParams.ShouldSkipScript(scripts, idx) {
			subCtx.Logger().Logf("Skipping '%s'", scripts[idx])
			continue
		}
		if scripts.IsScript(idx) {
			script, err := scripts.GetScript(idx)
			if err != nil {
				return errors.Wrapf(err, "failed to get script %d for step %s", idx, stepName)
			}
			pipelines.Logger().Logf("+ %s", script)
			_, err = dockerRun.ExecCommand(&runCtx, script)
			if err != nil {
				return errors.Wrapf(err, "failed to execute script '%s' for step %s", script, stepName)
			}
		} else if scripts.IsPipe(idx) {
			pipeDef, err := scripts.GetPipe(idx)
			if err != nil {
				return errors.Wrapf(err, "failed to get pipe %d for step %s", idx, stepName)
			}
			pipe := NewPipe(pipeDef.Pipe, pipelines.BitbucketContext)
			pipe.Variables = pipeDef.Variables.ToEnv()
			err = pipe.Run(&runCtx)
			if err != nil {
				return errors.Wrapf(err, "failed to execute pipe %s for step %s", pipeDef.Pipe, stepName)
			}
		} else {
			return errors.Errorf("unsupported script type %T", scripts[idx])
		}
	}
	if err := dockerRun.Destroy(); err != nil {
		return errors.Wrapf(err, "failed to destroy docker container for step %s", stepName)
	}
	err = stdout.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to close output channel for log streaming while running %s", stepName)
	}
	err = stderr.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to close error channel for log streaming while running %s", stepName)
	}
	return eg.Wait()
}

func (pipelines *BitbucketPipelines) runParallel(parallel schema.Parallel, runParams *BitbucketPipelinesRunParams) error {
	for _, step := range parallel.Parallel {
		if err := pipelines.StartParallel(func() error {
			if err := pipelines.runStep(step, runParams); err != nil {
				return errors.Wrapf(err, "failed to run step %s", *step.Step.Name)
			}
			return nil
		}); err != nil {
			return errors.Wrapf(err, "failed to start parallel execution")
		}
	}
	return nil
}

func (pipelines *BitbucketPipelines) Config() *schema.BitbucketPipelinesSchemaJson {
	return &pipelines.config
}
