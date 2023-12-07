package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/containerd/continuity/fs"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/system"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v3"
	"github.com/pkg/errors"
	"github.com/segmentio/textio"
	"github.com/smecsia/welder/pkg/docker/dockerext"
	"github.com/smecsia/welder/pkg/util"
	"golang.org/x/sync/errgroup"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	LabelNameContainerID     = "WelderBuildContainerID"
	LabelNameConfigHash      = "WelderBuildContainerConfigHash"
	HostSystemHostname       = "host.docker.internal" // hostname to access host machine (as of https://docs.docker.com/docker-for-mac/networking/)
	GatewayHostname          = "gateway"              // hostname to access gateway (in Linux it'd be the same as host machine, in Mac it'd be a host of Docker VM)
	DefaultContainerCommand  = "sleep 100000"
	DefaultContainerUser     = "root"
	ValidUsernameRegexString = `^[a-z_]([a-z0-9_-]{0,31}|[a-z0-9_-]{0,30}\$)$`
	DockerSockPath           = "/var/run/docker.sock"
	DefaultStopTimeout       = 1 * time.Second
	DefaultRunID             = "run"
)

var ValidUsernameRegex = regexp.MustCompile(ValidUsernameRegexString)

// NewRun creates new Run instance with the default client
func NewRun(runID string, ref string) (*Run, error) {
	dockerAPI, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	dockerAPI.NegotiateAPIVersion(ctx)
	dockerUtil, err := NewDockerUtil(dockerAPI, ctx)
	if err != nil {
		return nil, err
	}
	if runID == "" {
		runID = DefaultRunID
	}
	volumeApproach := VolumeApproachBind
	if dockerUtil.IsRunningInDocker() || dockerUtil.IsDockerHostRemote() {
		volumeApproach = VolumeApproachAdd
	}
	return &Run{
		RunID:          runID,
		Reference:      ref,
		dockerAPI:      dockerAPI,
		dockerUtil:     dockerUtil,
		volumeApproach: volumeApproach,
		osDistribution: UnknownOSDistribution, // by default, we don't know the distribution (should be detected later)
	}, nil
}

// Run prepares container and executes commands
func (run *Run) Run(runCtx RunContext, commands ...string) error {
	containerID, err := run.PrepareContainer(runCtx)

	if err != nil {
		return errors.Wrapf(err, "failed to prepare container")
	}

	if !runCtx.Detached {
		runCtx.Debugf("running in detached mode (commands will run in background)")
		// closing provided stdout if it's not system stdout
		if runCtx.Stdout != os.Stdout && runCtx.Stdout != nil {
			defer func(Stdout io.WriteCloser) {
				_ = Stdout.Close()
			}(runCtx.Stdout)
		}

		// closing provided stderr if it's not system stderr
		if runCtx.Stderr != os.Stderr && runCtx.Stderr != nil {
			defer func(Stderr io.WriteCloser) {
				_ = Stderr.Close()
			}(runCtx.Stderr)
		}
		// if we are not going to reuse containers, we need to remove them after execution
		if !run.reuseContainers {
			defer func(run *Run) {
				_ = run.Destroy()
			}(run)
		}
	}

	// if before executing commands hook is specified
	if runCtx.RunBeforeExec != nil {
		runCtx.Debugf("before exec hook is set, executing against container %s", containerID)
		err := runCtx.RunBeforeExec(containerID)

		if err != nil {
			return errors.Wrapf(err, "failed to exec before hook")
		}
	}

	executeCommandsFnc := func() error {
		if run.useDefaultCommand && len(commands) == 0 && !runCtx.Detached {
			// if no commands were provided, just wait until the default command finishes
			if err := run.Util().WaitUntilContainerExits(containerID); err != nil {
				return errors.Wrapf(err, "failed to wait for container %s", containerID)
			}
			if err := run.copyVolumesFromContainerIfNecessary(&runCtx); err != nil {
				return errors.Wrapf(err, "failed to copy volumes from container %s", containerID)
			}
			status, err := run.Util().GetContainerStatus(containerID)
			if err != nil {
				return errors.Wrapf(err, "failed to get container's status %s", containerID)
			}
			if status.ExitCode != 0 && runCtx.ErrorOnExitCode {
				return errors.Errorf("container %s exited with code %d", containerID, status.ExitCode)
			}
		}
		// run all commands inside container in the specified order
		for _, cmd := range commands {
			if _, err := run.ExecCommand(&runCtx, cmd); err != nil {
				return err
			}
		}
		// if after executing commands hook is specified
		if runCtx.RunAfterExec != nil {
			runCtx.Debugf("after exec hook is set, executing against container %s", containerID)
			err := runCtx.RunAfterExec(containerID)

			if err != nil {
				return errors.Wrapf(err, "failed to exec after hook")
			}
		}
		return nil
	}

	if runCtx.Detached {
		go func() {
			_ = executeCommandsFnc()
		}()
		return nil
	}
	return executeCommandsFnc()
}

func (run *Run) PrepareContainer(runCtx RunContext) (string, error) {
	// get hash of the configuration
	configHash, err := run.calcConfigHash(runCtx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to calc config hash")
	}
	run.initialConfigHash = configHash

	// search for existing container with the same runID
	// TODO: allow cleanup of all orphans with welder cleanup
	// TODO: make sure ssh hacks work with existing container on MacOS
	containerID, err := run.checkExistingContainer()
	if err != nil {
		runCtx.Debugf("existing container not found: %s", err.Error())
	}

	// if cleanup orphans is configured or reuse containers is not enabled
	if run.cleanupOrphans || !run.reuseContainers {
		runCtx.Debugf("removing orphans due to requested cleanup / reuse containers not enabled")
		// make sure we don't have orphans from previous run
		if err := run.Destroy(); err != nil {
			return containerID, err
		}
	}

	if !run.reuseContainers || containerID == "" {
		runCtx.Debugf("Creating new container. Existing was not found or reusing disabled!")
		containerID, err = run.createContainer(runCtx)
		if err != nil {
			return containerID, errors.Wrapf(err, "failed to create container")
		}
	} else {
		runCtx.Debugf("re-using existing container %s, trying to re-apply tweaks", containerID)
		// detecting OS distribution from a running container
		run.osDistribution = run.Util().DetectOSDistributionFromContainer(containerID)
		runCtx.Debugf("Detected OS distribution from the image: %s", run.osDistribution)
		// re-apply some existing integrations
		err = run.extraIntegrationsExistingContainer(runCtx, containerID)
		if err != nil {
			return containerID, errors.Wrapf(err, "failed to apply extra integrations to container: %s", containerID)
		}
	}

	// if reuse containers is not enabled, cleanup after process has exited
	if !run.reuseContainers {
		runCtx.Debugf("reuse containers disabled or existing config differs")
		run.destroyOnTermSignals(runCtx)
	}

	// container was found or created, set it as currently used one
	run.containerID = containerID
	return containerID, nil
}

func (run *Run) createContainer(runCtx RunContext) (string, error) {
	// make sure base image is pulled to host
	if err := run.makeSureImagePulled(runCtx); err != nil {
		return "", errors.Wrapf(err, "failed to pull image")
	}

	// calc extra system integrations
	tweaks := run.extraSystemIntegrations(runCtx)

	var imageID = run.Reference
	var err error
	if run.osDistribution.IsLinuxBased() {
		// building custom build image on top of provided Docker reference
		imageID, err = run.prepareBuildImage(runCtx, tweaks)
		if err != nil {
			return "", errors.Wrapf(err, "failed to build new Docker image on top of %s", run.Reference)
		}
	}

	// process provided config and return necessary binds
	runCtx.Debugf("configured volumes for this Docker run: binds=%v, mounts=%v", run.volumeBinds, run.volumeMounts)
	binds := run.configureBinds(runCtx, tweaks)
	mounts := run.configureVolumes(runCtx, tweaks)
	binds = append(binds, tweaks.extraBinds...)

	// parse specified port specs
	exposedPorts, exposedPortBinds, err := ParsePortsSpecs(run.ports)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse specified exposed ports: %v", run.ports)
	}

	// add extra environment variables if needed
	env := run.envVars
	env = append(env, tweaks.extraEnv...)
	env = append(env, runCtx.Env...)

	config := &container.Config{
		Image: imageID,
		Labels: map[string]string{
			LabelNameContainerID: run.RunID,
			LabelNameConfigHash:  run.initialConfigHash,
		},
		Env:          env,
		ExposedPorts: exposedPorts,
		WorkingDir:   runCtx.WorkDir,
	}

	config.User = DefaultContainerUser
	if runCtx.User != "" {
		config.User = runCtx.User
	}
	imgInspect, err := run.Util().InspectDockerImage(imageID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to inspect image %s", imageID)
	}

	// assuming we're dealing with linux-based image
	config.Entrypoint = []string{"/bin/sh"}
	config.Cmd = append([]string{"-c"}, DefaultContainerCommand)
	if run.entrypoint != nil {
		config.Entrypoint = run.entrypoint
	}
	if len(run.command) != 0 {
		config.Cmd = run.command
	}
	// if we are going to run non-Linux based image, we need to fall back to default command and entrypoint
	// if it was explicitly requested to use default command, we need to use default entrypoint as well
	if !run.osDistribution.IsLinuxBased() || run.useDefaultCommand {
		if run.entrypoint == nil {
			// if entrypoint is not set, use default one
			if imgInspect.Config.Entrypoint != nil {
				config.Entrypoint = runCtx.cleanupContainerCommand(imgInspect.Config.Entrypoint)
			} else {
				config.Entrypoint = runCtx.cleanupContainerCommand(imgInspect.ContainerConfig.Entrypoint)
			}
		}
		if len(run.command) == 0 {
			// if command is not set, use default one
			if len(imgInspect.Config.Cmd) > 0 {
				config.Cmd = runCtx.cleanupContainerCommand(imgInspect.Config.Cmd)
			} else {
				config.Cmd = runCtx.cleanupContainerCommand(imgInspect.ContainerConfig.Cmd)
			}
			run.useDefaultCommand = true
		}
	}

	// if we are going to run non-Linux based image, we need to fall back to default user
	if !run.osDistribution.IsLinuxBased() || run.useDefaultUser {
		runCtx.Debugf("WARN: Setting container user is not supported on %s (or was requested not to do so)", run.osDistribution.Name())
		config.User = imgInspect.ContainerConfig.User
	}

	hostConfig := &container.HostConfig{
		NetworkMode:  container.NetworkMode("default"),
		Privileged:   run.privileged,
		Binds:        binds,
		Mounts:       mounts,
		PortBindings: exposedPortBinds,
	}
	networkConfig := &network.NetworkingConfig{}

	ctx := run.GoContext()

	// generate unique name to not conflict with other running containers with the same runID
	cname := fmt.Sprintf("%s-%s", run.RunID, shortuuid.New()[:5])

	// run container config overrides if necessary
	if err := run.preConfigureContainer(config, hostConfig, networkConfig, tweaks.containerCreateTweaks...); err != nil {
		return "", errors.Wrapf(err, "failed to preconfigure container")
	}

	runCtx.Debugf("creating container with configuration %s, CMD@%s: %q", config, config.User, config.Cmd)
	createResp, err := run.dockerAPI.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, cname)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create container: %s", err.Error())
	}

	runCtx.Debugf("starting container %q", createResp.ID)
	// Start the background long-running sleep process in container
	if err := run.dockerAPI.ContainerStart(ctx, createResp.ID, types.ContainerStartOptions{}); err != nil {
		return "", errors.Wrapf(err, "failed to start container: %s", err.Error())
	}

	// if running in "detached" mode, streaming everything to the provided Stdout/Stderr (if provided)
	// same when we're not using sleep command and rely on the image default command
	if (runCtx.Detached || run.useDefaultCommand) && runCtx.Stdout != nil && runCtx.Stderr != nil {
		go func() {
			_ = run.Util().StreamContainerLogsTo(createResp.ID, runCtx.Stdout, runCtx.Stderr)
		}()
	}

	// if running a non-Linux based image, we don't build custom image, so we can't use ADD for volumes
	if !run.osDistribution.IsLinuxBased() && (run.volumeApproach == VolumeApproachAdd || run.volumeApproach == VolumeApproachCopy) {
		runCtx.Warnf("Volume approach 'add' and 'copy' are not supported on container with distribution %q, falling back to %s",
			run.osDistribution.Name(), VolumeApproachBind)
		run.volumeApproach = VolumeApproachBind
	}
	// prepare volumes in container if needed
	if run.volumeApproach != VolumeApproachAdd && run.volumeApproach != VolumeApproachExternal {
		// no need to prepare volumes if all files were added to the image already
		if err := run.prepareVolumesInContainer(runCtx, createResp.ID); err != nil {
			return "", errors.Wrapf(err, "failed to prepare volumes in container")
		}
	}

	// run additional pre-actions
	if err := run.runPreActions(createResp.ID, tweaks.extraActions...); err != nil {
		return "", errors.Wrapf(err, "failed to run extra system actions")
	}

	// exec extra system init commands if needed
	if err := run.execCommands(createResp.ID, tweaks.extraInitCommands...); err != nil {
		return "", errors.Wrapf(err, "failed to run extra system commands")
	}

	return createResp.ID, nil
}

func (run *Run) prepareVolumesInContainer(runCtx RunContext, containerID string) error {
	// copy volumes contents into container if running inside Docker/Kube environment (workaround volumes issue)
	if run.volumeApproach == VolumeApproachCopy {
		runCtx.Debugf("copyInsteadOfBind mode activated: copying volumes into container!")
		if err := run.copyAllVolumeBindsToContainer(runCtx, containerID); err != nil {
			return errors.Wrapf(err, "failed to copy all volumes into container")
		}
	}
	// create all volume mounts and change ownership to the proper user
	for _, v := range run.volumeMounts {
		runCtx.Debugf("Pre-creating volumes inside the container")
		rootCtx := runCtx.CloneAs("root", "/")
		mkdirCmd := "mkdir -p " + v.ContPath
		if runCtx.User != "" && runCtx.User != "root" {
			mkdirCmd += " && chown -R " + runCtx.VerboseFlag("-v") + " " + runCtx.User + ":" + runCtx.User + " " + v.ContPath
		}
		if _, err := run.execSingleCommand(containerID, svcCommand(rootCtx, mkdirCmd)); err != nil {
			return err
		}
	}
	return nil
}

func (run *Run) configureBinds(runCtx RunContext, tweak *extraTweak) []string {
	// make binds out of volumes if not running in container
	var binds []string

	for _, v := range run.volumeMounts {
		runCtx.Debugf("adding shared volume mount bind %v", v)
		binds = append(binds, fmt.Sprintf("%s:%s:z", v.Name, v.ContPath))
	}

	return binds
}

func (run *Run) configureVolumes(runCtx RunContext, tweak *extraTweak) []mount.Mount {
	run.AddVolumeBinds(tweak.extraVolumes...)

	// resolve host paths
	for idx, v := range run.volumeBinds {
		run.volumeBinds[idx].HostPath = run.resolveVolumeHostPath(runCtx, v)
	}

	// add volume binds if needed
	mounts := make([]mount.Mount, 0)
	if run.volumeApproach == VolumeApproachBind {
		for _, v := range run.volumeBinds {
			mounts = append(mounts, mount.Mount{
				Type:        mount.TypeBind,
				Source:      v.HostPath,
				Target:      v.ContPath,
				ReadOnly:    !v.Mode.IsRW(),
				Consistency: mount.Consistency(v.Mode.ConsistencyMode()),
			})
		}
	}
	runCtx.Debugf("Processed mounts: %s", mounts)
	return mounts
}

func (run *Run) resolveVolumeHostPath(runCtx RunContext, v Volume) string {
	updatedHostPath := v.HostPath
	if !filepath.IsAbs(updatedHostPath) {
		cwd, err := os.Getwd()
		if err != nil {
			runCtx.Warnf("WARN: Failed to determine work dir: ", err.Error())
		}
		runCtx.Debugf("Volume path %s is relative, joining with current dir: %s", updatedHostPath, cwd)
		updatedHostPath = filepath.Join(cwd, updatedHostPath)
	}
	runCtx.Debugf("Resolving symlinks for volume %s", updatedHostPath)
	resolvedHostPath, err := filepath.EvalSymlinks(updatedHostPath)
	if err != nil {
		runCtx.Warnf("WARN: Failed to eval symlinks: %s", err.Error())
	} else {
		runCtx.Debugf("Resolved symlinks for volume %s: %s", updatedHostPath, resolvedHostPath)
		updatedHostPath = resolvedHostPath
	}
	return updatedHostPath
}

// Destroy removes all containers with their volumes of a current run session
func (run *Run) Destroy() error {
	_ = run.cleanupNetworks()
	_ = run.cleanupContainers()
	return nil
}

// ExecWithOutput Simple execute to run command and return output
func (run *Run) ExecWithOutput(command string) (string, error) {
	return run.Util().ExecInContainer(run.ContainerID(), command)
}

// ExecCommand executes command same way as docker_run
func (run *Run) ExecCommand(runCtx *RunContext, command string) (ExecResult, error) {
	var copyErr error
	execRes, execErr := run.execSingleCommand(run.ContainerID(), ExecContext{
		runCtx: runCtx, command: command, serviceCmd: false, ignoreErrors: false, detach: false,
	})
	if run.keepEnvVariables { // keep environment variables if requested
		runCtx.Env = util.SliceDistinct(append(execRes.Env, runCtx.Env...))
	}
	copyErr = run.copyVolumesFromContainerIfNecessary(runCtx)
	if execErr != nil {
		return execRes, execErr
	}
	if copyErr != nil {
		return execRes, copyErr
	}
	return execRes, nil
}

func (run *Run) copyVolumesFromContainerIfNecessary(runCtx *RunContext) error {
	var copyErr error
	// copy rw volumes contents from container if running in dind environment (sync volumes back to host)
	if run.volumeApproach == VolumeApproachCopy || run.volumeApproach == VolumeApproachAdd {
		runCtx.Debugf("Docker In Docker mode activated: copying volumes from container!")
		copyErr = run.copyAllVolumesFromContainer(runCtx, run.ContainerID())
	}
	return copyErr
}

// prepareBuildImage builds new Docker image for build based on provided image reference
func (run *Run) prepareBuildImage(runCtx RunContext, tweak *extraTweak) (string, error) {
	imageID, err := run.findExistingBuildImage(runCtx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find existing image")
	}
	if imageID != "" && !run.disableCache {
		return imageID, nil
	} else if run.disableCache {
		runCtx.Debugf("Use of cached build image is disabled")
	}
	runCtx.Debugf("Existing image with the same hash not found, building new one!")

	var dockerFileContents strings.Builder
	dockerFileContents.WriteString("FROM " + run.Reference + "\n")
	dockerFileContents.WriteString("USER root\n")
	dockerFileContents.WriteString("RUN set -e; \\ \n")

	// remove symlinks that are targeted by volumes
	// see https://github.com/docker/docker/issues/17944 for more info
	for _, v := range run.volumeBinds {
		dockerFileContents.WriteString(`[ -L "` + v.ContPath + `" ] && [ -e "` + v.ContPath + `" ] && rm -f ` + v.ContPath + " || true; \\ \n")
		srcInfo, _ := os.Stat(v.HostPath)
		// pre-create parent directory for files for external sync tool
		if srcInfo != nil && !srcInfo.IsDir() && run.volumeApproach == VolumeApproachExternal {
			dir := path.Dir(v.ContPath)
			dockerFileContents.WriteString(`mkdir -p "` + dir + `" || true; \\ \n`)
		}
	}
	for _, v := range run.volumeMounts {
		dockerFileContents.WriteString(`[ -L "` + v.ContPath + `" ] && [ -e "` + v.ContPath + `" ] && rm -f ` + v.ContPath + " || true; \\ \n")
	}

	// add extra commands
	for _, command := range tweak.extraBuildCommands {
		dockerFileContents.WriteString(command + "; \\ \n")
	}

	dockerFileContents.WriteString(`echo OK;`)

	tmpDir, err := ioutil.TempDir("", run.RunID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tmpDir)
	dockerFilePath := path.Join(tmpDir, "Dockerfile")

	// if volumes should be added to the image instead of container
	if run.volumeApproach == VolumeApproachAdd {
		if err := run.addVolumeBindsToBuildImageCommands(runCtx, &dockerFileContents, dockerFilePath); err != nil {
			return "", err
		}
	}

	runCtx.Debugf(dockerFileContents.String())
	err = ioutil.WriteFile(dockerFilePath, []byte(dockerFileContents.String()), os.ModePerm)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write temp dockerfile")
	}
	refParts := reference.ReferenceRegexp.FindStringSubmatch(run.Reference)
	refName := refParts[1]
	refTag := refParts[2]
	if refTag == "" {
		refTag = "latest"
	}
	buildImgTag := strings.ToLower(fmt.Sprintf("ab-%s-%s:%s", refName, run.RunID, refTag))
	runCtx.Debugf("Going to tag build image as %q", buildImgTag)
	dockerFile, err := NewDockerfile(run.GoContext(), dockerFilePath, buildImgTag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to init dockerfile")
	}
	dockerFile.Context = run.GoContext()
	dockerFile.DisableNoCache = !run.disableCache
	dockerFile.ReuseImagesWithSameCfg = !run.disableCache
	dockerFile.Labels = map[string]string{
		LabelNameContainerID: run.RunID,
		LabelNameConfigHash:  run.initialConfigHash,
	}
	for k, v := range tweak.extraContainerLabels {
		dockerFile.Labels[k] = v
	}
	dockerFile.DisablePull = true

	reader, err := dockerFile.Build()
	if err != nil {
		return "", errors.Wrapf(err, "failed to build new docker image")
	}
	if err := reader.Listen(false, MessageToLogFunc(runCtx.logger(true), "")); err != nil {
		return "", err
	}
	return dockerFile.ID(), nil
}

func (run *Run) addVolumeBindsToBuildImageCommands(runCtx RunContext, dockerFileContents *strings.Builder, dockerFilePath string) error {
	dockerFileDir := filepath.Dir(dockerFilePath)
	ctxSubDir := filepath.Join(dockerFileDir, "tmp")
	if err := os.MkdirAll(ctxSubDir, os.ModePerm); err != nil {
		return err
	}
	for _, v := range run.volumeBinds {
		hostPathName := shortuuid.New()[:10]
		tmpHostPath := filepath.Join(ctxSubDir, hostPathName)
		srcInfo, err := os.Stat(v.HostPath)
		if err != nil {
			if os.IsNotExist(err) {
				return errors.Wrapf(err, "path %s does not exist in the host container", v.HostPath)
			} else if os.IsPermission(err) {
				return errors.Wrapf(err, "path %s is not accessible in the host container", v.HostPath)
			}
		}
		if srcInfo.IsDir() {
			xAttrErrHandler := fs.WithXAttrErrorHandler(func(dst, src, xattrKey string, err error) error {
				// security.* xattr cannot be copied in most cases (moby/buildkit#1189)
				runCtx.Warnf("failed to copy xattr %q: %s", xattrKey, err.Error())
				return nil
			})
			if err := fs.CopyDir(tmpHostPath, v.HostPath, xAttrErrHandler); err != nil {
				return errors.Wrapf(err, "failed to prepare a copy of volume dir %s to %s", v.HostPath, tmpHostPath)
			}
		} else {
			if err := fs.CopyFile(tmpHostPath, v.HostPath); err != nil {
				return errors.Wrapf(err, "failed to prepare a copy of volume file %s", v.HostPath)
			}
		}
		if relativeHostPath, err := filepath.Rel(dockerFileDir, tmpHostPath); err != nil {
			return err
		} else {
			dockerFileContents.WriteString(fmt.Sprintf("\nADD %s %s", relativeHostPath, v.ContPath))
			if runCtx.User != "" && runCtx.User != "root" {
				dockerFileContents.WriteString(fmt.Sprintf("\nRUN " + fmt.Sprintf("chown -R %s %s:%s %s || true", runCtx.VerboseFlag("-v"),
					runCtx.User, runCtx.User, v.ContPath)))
			}
		}
	}
	return nil
}

func (run *Run) findExistingBuildImage(runCtx RunContext) (string, error) {
	runCtx.Debugf("Checking if there is an existing image with same hash %s...", run.initialConfigHash)
	allImages, err := run.dockerAPI.ImageList(run.GoContext(), types.ImageListOptions{All: true})
	if err != nil {
		return "", errors.Wrapf(err, "failed to list images from Docker")
	}
	for _, image := range allImages {
		if image.Labels[LabelNameConfigHash] == run.initialConfigHash {
			runCtx.Debugf("Reusing existing image with the same hash: %s", image.ID)
			return image.ID, nil
		}
	}
	return "", nil
}

func (run *Run) copyAllVolumesFromContainer(runCtx *RunContext, containerID string) error {
	return run.copyVolumesFromContainer(runCtx, containerID, run.volumeBinds)
}

func (run *Run) copyVolumesFromContainer(runCtx *RunContext, containerID string, volumes []Volume) error {
	// get all changed paths since container started (will return all sub directories / files of all volumes)
	diffPaths, err := run.dockerAPI.ContainerDiff(run.GoContext(), containerID)
	if err != nil {
		return errors.Wrapf(err, "failed to get diff for container %s", containerID)
	}
	if runCtx.Debug {
		for _, diffPath := range diffPaths {
			runCtx.Debugf("Container diff path: %s", diffPath)
		}
	}
	for _, v := range volumes {
		// copying only RW volumes back to host
		if !v.Mode.IsRW() {
			continue
		}
		for _, diffPath := range diffPaths {
			// check whether change is a part of volume or the volume itself (in case of file)
			partOfVolume := strings.HasPrefix(diffPath.Path, path.Clean(v.ContPath)+"/") || diffPath.Path == path.Clean(v.ContPath)
			if partOfVolume {
				contPath := diffPath.Path
				relPath, err := filepath.Rel(path.Clean(v.ContPath), contPath)
				if err != nil {
					return errors.Wrapf(err, "failed to calculate relative path of changed container mount %s off %s", diffPath.Path, v.ContPath)
				}
				// skip if file/dir is not a direct child of a volume
				if strings.Contains(relPath, "/") || contPath == v.ContPath {
					continue
				}
				if err := run.copyFromContainer(runCtx, containerID, contPath, v.HostPath); err != nil {
					return errors.Wrapf(err, "could not copy file %s of volume %s:%s back from container", relPath, v.HostPath, v.ContPath)
				}
			}
		}
	}
	return nil
}

// preConfigureContainer runs extra tweaks before creating container
func (run *Run) preConfigureContainer(config *container.Config, hostConfig *container.HostConfig, netConfig *network.NetworkingConfig, actions ...containerCreateTweak) error {
	for _, action := range actions {
		action(config, hostConfig, netConfig)
	}
	return nil
}

// runPreActions runs additional actions before starting commands
func (run *Run) runPreActions(containerID string, actions ...tweakAction) error {
	for _, action := range actions {
		action(run.dockerAPI, containerID)
	}
	return nil
}

// execCommands executes list of exec contexts
func (run *Run) execCommands(containerID string, commands ...ExecContext) error {
	for _, cmd := range commands {
		_, err := run.execSingleCommand(containerID, cmd)
		if err != nil {
			return errors.Wrapf(err, "failed to execute command %s", cmd.command)
		}
	}
	return nil
}

func (run *Run) copyAllVolumeBindsToContainer(runCtx RunContext, containerID string) error {
	return run.copyVolumeBindsToContainer(runCtx, containerID, run.volumeBinds)
}

func (run *Run) copyVolumeBindsToContainer(runCtx RunContext, containerID string, volumes []Volume) error {
	for _, v := range volumes {
		if v.HostPath == DockerSockPath || strings.HasSuffix(v.HostPath, ".sock") {
			continue
		}
		if err := run.copyToContainer(runCtx, containerID, v.HostPath, v.ContPath); err != nil {
			return errors.Wrapf(err, "could not copy volume bind %s:%s to container %s", v.HostPath, v.ContPath, containerID)
		}
	}
	return nil
}

func (run *Run) copyFromContainer(runCtx *RunContext, containerID string, contPath string, hostPath string) error {
	runCtx.Debugf("COPY %s:%s %s", containerID, contPath, hostPath)
	// if client requests to follow symbol link, then must decide target file to be copied
	var rebaseName string
	contStat, err := run.dockerAPI.ContainerStatPath(run.GoContext(), containerID, contPath)

	// If the source is a symbolic link, we should follow it.
	if err == nil && contStat.Mode&os.ModeSymlink != 0 {
		linkTarget := contStat.LinkTarget
		if !system.IsAbs(linkTarget) {
			// Join with the parent directory.
			srcParent, _ := archive.SplitPathDirEntry(contPath)
			linkTarget = filepath.Join(srcParent, linkTarget)
		}

		linkTarget, rebaseName = archive.GetRebaseName(contPath, linkTarget)
		contPath = linkTarget
	}

	content, stat, err := run.dockerAPI.CopyFromContainer(run.GoContext(), containerID, contPath)
	if err != nil {
		return err
	}
	defer content.Close()

	if hostPath == "-" {
		// Send the response to STDOUT.
		_, err = io.Copy(runCtx.Stdout, content)

		return err
	}

	// Prepare source copy info.
	srcInfo := archive.CopyInfo{
		Path:       contPath,
		Exists:     true,
		IsDir:      stat.Mode.IsDir(),
		RebaseName: rebaseName,
	}

	preArchive := content
	if len(srcInfo.RebaseName) != 0 {
		_, srcBase := archive.SplitPathDirEntry(srcInfo.Path)
		preArchive = archive.RebaseArchiveEntries(content, srcBase, srcInfo.RebaseName)
	}
	// See comments in the implementation of `archive.CopyTo` for exactly what
	// goes into deciding how and whether the source archive needs to be
	// altered for the correct copy behavior.
	return archive.CopyTo(preArchive, srcInfo, hostPath)
}

func (run *Run) copyToContainer(runCtx RunContext, containerID string, hostPath string, contPath string) error {
	runCtx.Debugf("COPY %s %s:%s", hostPath, containerID, contPath)
	srcInfo, err := os.Lstat(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Wrapf(err, "path %s does not exist in the host container", hostPath)
		} else if os.IsPermission(err) {
			return errors.Wrapf(err, "path %s is not accessible in the host container", hostPath)
		}
	}
	hostPath, err = filepath.EvalSymlinks(hostPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read path behind symlink %s", hostPath)
	}
	srcFileName := path.Base(hostPath)
	dstFileName := path.Base(contPath)
	dstDirPath := contPath
	if !srcInfo.IsDir() {
		dstDirPath = path.Dir(contPath)
	}

	rootCtx := runCtx.CloneAs("root", "/")
	preCreateTargetDir := func() error {
		mkdirCmd := "mkdir -p " + dstDirPath
		if runCtx.User != "" && runCtx.User != "root" {
			mkdirCmd += " && chown " + runCtx.VerboseFlag("-v") + " " + runCtx.User + ":" + runCtx.User + " " + dstDirPath
		}
		if _, err := run.execSingleCommand(containerID, svcCommand(rootCtx, mkdirCmd)); err != nil {
			return errors.Wrapf(err, "failed to pre-create target directory %s", dstDirPath)
		}
		return nil
	}

	if err := preCreateTargetDir(); err != nil {
		return err
	}

	var containerStatus = "unknown"
	if status, err := run.Util().GetContainerStatus(containerID); err != nil {
		return errors.Wrapf(err, "failed to get container's status %q", containerID)
	} else {
		containerStatus = fmt.Sprintf("(exists:%t,running:%t)", status.Exists, status.Running)
	}
	copyString := fmt.Sprintf("%s -> %s:%s", hostPath, containerID, dstDirPath)
	runCtx.Debugf("Container status: %s", containerStatus)
	runCtx.Debugf("Running copy to container %s", copyString)
	copyFnc := func() error {
		return run.Util().CopyToContainer(hostPath, containerID, dstDirPath)
	}

	if err := copyFnc(); err != nil && strings.Contains(err.Error(), "No such container:path") {
		// try to pre-create second time (to eliminate some weird flakiness when running on CI)
		if err := preCreateTargetDir(); err != nil {
			return err
		} else if err := copyFnc(); err != nil {
			return errors.Wrapf(err, "failed to copy %s after 2 attempts. status:%s",
				copyString, containerStatus)
		}
	} else {
		return errors.Wrapf(err, "failed to copy to container %s status:%s", copyString, containerStatus)
	}

	// if source is a file and its name differs from destination, we need to rename it on target
	if !srcInfo.IsDir() && dstFileName != srcFileName {
		renameCmd := svcCommand(rootCtx, "mv "+dstDirPath+"/"+srcFileName+" "+dstDirPath+"/"+dstFileName)
		if _, err := run.execSingleCommand(containerID, renameCmd); err != nil {
			return err
		}
	}

	// make sure running user has access to the destination
	if runCtx.User != "" && runCtx.User != "root" {
		chownCmd := svcCommand(rootCtx, fmt.Sprintf("chown -R %s %s:%s %s || true", runCtx.VerboseFlag("-v"),
			runCtx.User, runCtx.User, contPath))
		if _, err := run.execSingleCommand(containerID, chownCmd); err != nil {
			return err
		}
	}

	return nil
}

func (run *Run) cleanupImages() error {
	ctx := run.GoContext()
	images, err := run.dockerAPI.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		return err
	}
	var eg errgroup.Group
	for _, i := range images {
		image := i
		if image.Labels[LabelNameContainerID] == run.RunID {
			eg.Go(func() error {
				_, err := run.dockerAPI.ImageRemove(ctx, image.ID, types.ImageRemoveOptions{Force: true, PruneChildren: true})
				return err
			})
		}
	}
	err = eg.Wait()
	return err
}

func (run *Run) cleanupNetworks() error {
	ctx := context.Background()
	networks, err := run.dockerAPI.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return err
	}
	// remove all created networks (by label)
	for _, net := range networks {
		n := net
		if n.Labels[LabelNameContainerID] == run.RunID {
			// disconnect all existing containers from network
			networkInfo, _ := run.dockerAPI.NetworkInspect(ctx, net.Name, types.NetworkInspectOptions{Verbose: true})
			for _, c := range networkInfo.Containers {
				_ = run.dockerAPI.NetworkDisconnect(ctx, net.ID, c.Name, true)
			}
			_ = run.dockerAPI.NetworkRemove(ctx, n.ID)
		}
	}
	// clean up networks of container
	//containerID, _ := run.checkExistingContainer()
	//_ = run.Util().CleanupContainerNetworks(containerID)
	return nil
}

func (run *Run) cleanupContainers() error {
	containers, err := run.dockerAPI.ContainerList(run.GoContext(), types.ContainerListOptions{All: true})
	if err != nil {
		return err
	}
	for _, cont := range containers {
		c := cont
		if c.Labels[LabelNameContainerID] == run.RunID {
			timeout := DefaultStopTimeout
			if run.stopTimeout > 0 {
				timeout = run.stopTimeout
			}
			return run.dockerUtil.ForceRemoveContainer(c.ID, timeout)
		}
	}
	return nil
}

func (run *Run) checkExistingContainer() (string, error) {
	containers, err := run.dockerAPI.ContainerList(run.GoContext(), types.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}
	for _, cont := range containers {
		c := cont
		if c.Labels[LabelNameConfigHash] == run.initialConfigHash {
			return c.ID, nil
		}
	}
	return "", errors.Errorf("container not found")
}

func (run *Run) makeSureImagePulled(runCtx RunContext) error {
	runCtx.Debugf("Making sure image is pulled: %s", run.Reference)
	// parsing reference to understand what's been requested
	ref, err := name.ParseReference(run.Reference, name.WeakValidation)
	argsFilter := filters.Arg("reference", run.Reference)
	if tag, ok := ref.(name.Tag); ok {
		// if provided reference doesn't end with a tag, add ":latest" to the filter
		if !strings.HasSuffix(run.Reference, ":"+tag.TagStr()) {
			argsFilter = filters.Arg("reference", run.Reference+":latest")
		}
	}

	opts := types.ImageListOptions{
		Filters: filters.NewArgs(argsFilter),
	}
	ctx := run.GoContext()
	images, err := run.dockerAPI.ImageList(ctx, opts)
	if err != nil {
		return err
	}
	if len(images) == 0 {
		// pull image for current platform
		err := run.pullImageForPlatformAndWait(runCtx, runtime.GOARCH)
		if err != nil {
			// special handling for arm64 platform, since there might be not many images for arm64, falling back to amd64
			if runtime.GOARCH == `arm64` {
				runCtx.Debugf("We're running on arm64 platform and this error happened: %s", err.Error())
				err = run.pullImageForPlatformAndWait(runCtx, `amd64`)
			}
			if err != nil {
				return err
			}
		}
	}

	// detect os distribution after the base image is pulled
	run.osDistribution = run.Util().DetectOSDistributionFromImage(run.Reference)
	runCtx.Debugf("Detected OS distribution from the image: %s", run.osDistribution)
	return nil
}

func (run *Run) pullImageForPlatformAndWait(runCtx RunContext, platform string) error {
	var eg errgroup.Group
	// pull image
	dockerMsgReader := chanMsgReader{msgChan: make(chan readerNextMessage)}

	runCtx.Debugf("Resolving registry from reference: %s", run.Reference)
	registry, err := RegistryFromImageReference(run.Reference)
	if err != nil {
		return err
	}

	runCtx.Debugf("Pulling image %s with proper auth: '%s' for platform: '%s'",
		run.Reference, registry.AuthHeader, platform)
	reader, err := run.dockerAPI.ImagePull(run.GoContext(), run.Reference, types.ImagePullOptions{
		RegistryAuth: registry.AuthHeader,
		Platform:     platform,
	})
	if err != nil {
		return err
	}

	eg.Go(func() error {
		return run.streamMessagesToChannel(bufio.NewReader(reader), dockerMsgReader.msgChan)
	})

	if runCtx.Logger != nil {
		if err := dockerMsgReader.Listen(false, MessageToLogFunc(runCtx.Logger, runCtx.Prefix)); err != nil {
			return err
		}
	} else {
		if err := dockerMsgReader.Listen(false, func(message *ResponseMessage, err error) {
			if runCtx.Stdout != nil {
				_, _ = runCtx.Stdout.Write([]byte(runCtx.Prefix +
					strings.Replace(message.summary, "\n", "\n"+runCtx.Prefix, -1)))
			}
		}); err != nil {
			return err
		}
	}
	return eg.Wait()
}

func (run *Run) execSingleCommand(containerID string, cmd ExecContext) (ExecResult, error) {
	cmd.command = strings.TrimSpace(cmd.command)
	envFile := fmt.Sprintf("/tmp/%s.env", uuid.New().String())
	command := []string{cmd.command}
	if run.osDistribution.IsLinuxBased() { // on linux we can read the environment variables after command execution
		command = []string{"/bin/sh", "-c", fmt.Sprintf(`trap "env > %s" EXIT; %s`, envFile, cmd.command)}
	}
	execConfig := types.ExecConfig{
		Privileged:   run.privileged,
		Cmd:          command,
		AttachStdout: !cmd.doNotAttachStdOut,
		AttachStderr: !cmd.doNotAttachStdOut,
		AttachStdin:  !cmd.doNotAttachStdIn,
		Detach:       cmd.detach,
		Tty:          cmd.runCtx.Tty,
		Env:          cmd.runCtx.Env,
	}

	stdout := io.Writer(cmd.runCtx.Stdout)
	if stdout == nil {
		cmd.runCtx.Debugf("specified stdout is nil, fallback to os.Stdout!")
		stdout = os.Stdout
	}
	stderr := io.Writer(cmd.runCtx.Stderr)
	if stderr == nil {
		cmd.runCtx.Debugf("specified stderr is nil, fallback to os.Stderr!")
		stderr = os.Stderr
	}
	var stdin io.ReadCloser = nil
	if !cmd.serviceCmd {
		stdin = cmd.runCtx.Stdin
		if stdin == nil {
			cmd.runCtx.Debugf("specified stdin is nil, fallback to os.Stdin!")
			stdin = os.Stdin
		}
	}
	if !cmd.runCtx.Tty {
		stderr = textio.NewPrefixWriter(stderr, cmd.runCtx.Prefix)
		stdout = textio.NewPrefixWriter(stdout, cmd.runCtx.Prefix)
		stdin = nil
	}

	if cmd.runCtx.WorkDir != "" {
		execConfig.WorkingDir = cmd.runCtx.WorkDir
	}

	if cmd.runCtx.User != "" && ValidUsernameRegex.Match([]byte(cmd.runCtx.User)) {
		execConfig.User = cmd.runCtx.User
	}

	res := ExecResult{}
	ctx := run.GoContext()
	crResp, err := run.dockerAPI.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil && !cmd.ignoreErrors {
		return res, errors.Wrapf(err, "failed to create exec in container %s", containerID)
	}
	res.ExecID = crResp.ID

	if cmd.detach {
		cmd.runCtx.Debugf("CMD@%s[detached] `%s`", execConfig.User, cmd.command)
		return res, run.dockerAPI.ContainerExecStart(ctx, crResp.ID, types.ExecStartCheck{Detach: true})
	}

	cmd.runCtx.Debugf("CMD@%s `%s`", execConfig.User, cmd.command)

	if err := dockerext.InteractiveExec(run.dockerAPI, dockerext.InteractiveExecCfg{
		ExecID:     crResp.ID,
		Context:    run.GoContext(),
		Stderr:     stderr,
		Stdout:     stdout,
		Stdin:      stdin,
		ExecConfig: &execConfig,
	}); err != nil && !cmd.ignoreErrors {
		return res, errors.Wrapf(err, "failed to run interactive exec in container %s", containerID)
	}

	ceiResp, err := run.dockerAPI.ContainerExecInspect(ctx, crResp.ID)
	res.ExitCode = ceiResp.ExitCode
	res.Pid = ceiResp.Pid

	// read environment variables from the file after command execution (can only do on Linux containers)
	if run.osDistribution.IsLinuxBased() {
		if content, err := run.Util().ReadFileFromContainer(containerID, envFile); err != nil {
			cmd.runCtx.Warnf("failed to read environment variables from container: %s", err.Error())
		} else {
			res.Env = strings.Split(content, "\n")
		}
	}

	if err != nil && !cmd.ignoreErrors {
		return res, errors.Wrapf(err, "failed to inspect exec for container %s", containerID)
	}
	if !cmd.ignoreErrors && cmd.runCtx.ErrorOnExitCode && ceiResp.ExitCode != 0 {
		return res, errors.Errorf("command '%s' failed: exit code: %d", cmd.command, ceiResp.ExitCode)
	}
	return res, nil
}

func (run *Run) streamMessagesToChannel(reader *bufio.Reader, msgChan chan readerNextMessage) error {
	scanner := util.NewLineOrReturnScanner(reader)
	for {
		if !scanner.Scan() {
			msgChan <- readerNextMessage{EOF: true}
			return nil
		}
		line := string(scanner.Bytes())
		err := scanner.Err()
		if err != nil {
			msgChan <- readerNextMessage{Error: err}
		}
		msg := ResponseMessage{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			msgChan <- readerNextMessage{Error: err}
		} else {
			if msg.Error != "" {
				msgChan <- readerNextMessage{Error: errors.New(msg.Error)}
			} else {
				msgChan <- readerNextMessage{Message: msg}
			}
		}
	}
}

func (run *Run) destroyOnTermSignals(runCtx RunContext) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGKILL)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGINT)

	go func() {
		runCtx.Debugf("caught signal: %+v", <-signalCh)
		_ = run.Destroy()
	}()
}
