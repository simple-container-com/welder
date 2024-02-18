package runner

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/exec"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/types"
	"golang.org/x/sync/errgroup"
)

type Run struct {
	*types.CommonCtx
}

type RunParams struct {
	ProjectName string
	Volumes     []docker.Volume
	WorkDir     string
}

func NewRun(ctx *types.CommonCtx) *Run {
	return &Run{
		CommonCtx: ctx,
	}
}

func (ctx *Run) CalcRunInContainerParams(projectName string, projectRoot string, runCfg types.CommonRunDefinition, mainVolumeConfig *docker.Volume) (res *RunParams, err error) {
	if !filepath.IsAbs(projectRoot) {
		return nil, errors.Errorf("projectRoot must be an absolute path, got %q", projectRoot)
	}

	// Generate volumes
	volumes, err := runCfg.Volumes.ToDockerVolumes(ctx.CommonCtx)
	if err != nil {
		return nil, err
	}

	// host dir is always root
	hostWd := projectRoot

	containerWd := hostWd
	if runCfg.ContainerWorkDir != "" {
		containerWd = runCfg.ContainerWorkDir
	}

	// work dir can be redefined
	workDir := containerWd
	if runCfg.WorkDir != "" {
		if path.IsAbs(runCfg.WorkDir) {
			workDir = runCfg.WorkDir
		} else {
			workDir = path.Join(hostWd, runCfg.WorkDir)
		}
	}

	mainVolume := docker.Volume{
		HostPath: projectRoot,
		ContPath: containerWd,
		Mode:     "rw",
	}

	// pass main volume mode specified in "default" section
	if mainVolumeConfig != nil && mainVolumeConfig.Mode != "" {
		mainVolume.Mode = mainVolumeConfig.Mode
	} else if ctx.OS() != `linux` { // use "delegated" consistency for main volume by default on non-Linux
		mainVolume.Mode = docker.VolumeModeDelegated
	}

	// current directory a volume
	volumes = append(volumes, mainVolume)

	return &RunParams{
		Volumes:     volumes,
		WorkDir:     workDir,
		ProjectName: projectName,
	}, nil
}

func (ctx *Run) RunInContainer(action string, runID string, containerRunParams *RunParams, spec types.RunSpec) error {
	ctx.Logger().Logf(" - Running %d scripts in container '%s'...", len(spec.Scripts), spec.Image)
	var eg errgroup.Group
	dockerRun, err := docker.NewRun(runID, spec.Image)
	if err != nil {
		return errors.Wrapf(err, "failed to init docker build for %s with image %s", action, spec.Image)
	}
	if err := ctx.ConfigureVolumes(dockerRun, containerRunParams); err != nil {
		return errors.Wrapf(err, "failed to configure volumes")
	}
	dockerRun.
		SetReuseContainers(ctx.ReuseContainers).
		SetDisableCache(ctx.NoCache).
		SetCleanupOrphans(ctx.RemoveOrphans).
		SetContext(ctx.GoContext()).
		MountDockerSocket().
		KeepEnvironmentWithEachCommand()

	logReader, logStdOut := io.Pipe()
	logReaderErr, logStderr := io.Pipe()
	eg.Go(util.ReaderToLogFunc(logReader, false, "", ctx.Logger(), fmt.Sprintf("%s with image %s", action, spec.Image)))
	eg.Go(util.ReaderToLogFunc(logReaderErr, true, "ERR: ", ctx.Logger(), fmt.Sprintf("%s with image %s", action, spec.Image)))
	captReader, captOutput := io.Pipe()
	var captBuf bytes.Buffer
	eg.Go(util.ReaderToBufFunc(captReader, &captBuf))

	defer func() {
		ctx.SetLastExecOutput(string(captBuf.Bytes()))
	}()

	stdout := util.MultiWriteCloser(logStdOut, captOutput)
	stderr := util.MultiWriteCloser(logStderr, captOutput)

	scripts := spec.Scripts
	runCfg := docker.RunContext{
		Env:             spec.RunCfg.Env.ToBuildEnv(spec.RunCfg.InjectEnvRegex(ctx.CommonCtx)...),
		Stdout:          stdout,
		Stderr:          stderr,
		User:            ctx.Username,
		WorkDir:         containerRunParams.WorkDir,
		Debug:           ctx.Verbose,
		ErrorOnExitCode: true,
		RunBeforeExec: func(containerID string) error {
			// need to sync volumes into container before we can continue
			if ctx.SyncMode == types.SyncModeExternal {
				return ctx.SyncVolumesViaExternalTool(
					fmt.Sprintf("docker://%s@%s", ctx.Username, containerID),
					runID,
					containerRunParams)
			}
			return nil
		},
		RunAfterExec: func(containerID string) error {
			// need to terminate sync sessions that were created before
			if ctx.SyncMode == types.SyncModeExternal {
				return ctx.terminateExternalSyncSessions(runID, containerRunParams)
			}
			return nil
		},
		Logger: ctx.Logger(),
	}

	// if there were no commands provided to run, we run the default one specified in the Docker image
	if len(scripts) == 0 {
		dockerRun.UseDefaultCommand()
	}

	err = dockerRun.Run(runCfg, scripts...)
	if err != nil {
		return errors.Wrapf(err, "failed to run %s in Docker", action)
	}
	err = stdout.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to close output channel for log streaming while running %s", action)
	}
	return eg.Wait()
}

func (ctx *Run) RunOnHost(action string, runID string, containerRunParams *RunParams, spec types.RunSpec) error {
	ctx.Logger().Logf(" - Running %d scripts on host...", len(spec.Scripts))

	var captBuf bytes.Buffer
	defer func() {
		ctx.SetLastExecOutput(string(captBuf.Bytes()))
	}()

	executor := exec.NewExecWithOutput(ctx.GoContext(), ctx.Logger(), &captBuf)

	for _, script := range spec.Scripts {
		ctx.Logger().Logf(" - Executing script: '%s'", script)
		workDir := containerRunParams.WorkDir
		env := spec.RunCfg.Env.ToBuildEnv(spec.RunCfg.InjectEnvRegex(ctx.CommonCtx)...)
		if execRes, err := executor.ExecCommandAndLog(action, script, exec.Opts{
			Wd:  workDir,
			Env: env,
		}); err != nil {
			return errors.Wrapf(err, "failed to execute %q (%q)", action, script)
		} else {
			spec.RunCfg.Env = types.ParseBuildEnv(execRes.Env)
		}
	}
	return nil
}
