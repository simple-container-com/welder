package docker

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"runtime"
	"time"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/fatih/color"
	"github.com/simple-container-com/welder/pkg/util"
)

//
// Run helpers
//

func (run *Run) SetEnv(env ...string) *Run {
	run.envVars = env
	return run
}

func (run *Run) AddEnv(env ...string) *Run {
	run.envVars = append(run.envVars, env...)
	return run
}

func (run *Run) SetEntrypoint(entrypoint ...string) *Run {
	run.entrypoint = entrypoint
	return run
}

func (run *Run) SetCommand(command ...string) *Run {
	run.command = command
	return run
}

func (run *Run) SetStopTimeout(timeout time.Duration) *Run {
	run.stopTimeout = timeout
	return run
}

func (run *Run) EnableCleanupOrphans() *Run {
	run.SetCleanupOrphans(true)
	return run
}

func (run *Run) SetCleanupOrphans(val bool) *Run {
	run.cleanupOrphans = val
	return run
}

func (run *Run) AllowReuseContainers() *Run {
	run.SetReuseContainers(true)
	return run
}

func (run *Run) FallbackToApproachInsteadOfBind(approach VolumeApproach) *Run {
	run.SetVolumeApproach(VolumeApproachBind)
	if run.dockerUtil.IsRunningInDocker() || run.dockerUtil.IsDockerHostRemote() {
		run.SetVolumeApproach(approach)
	}
	return run
}

func (run *Run) KeepEnvironmentWithEachCommand() *Run {
	run.keepEnvVariables = true
	return run
}

func (run *Run) SetVolumeApproach(approach VolumeApproach) *Run {
	run.volumeApproach = approach
	return run
}

func (run *Run) SetReuseContainers(val bool) *Run {
	run.reuseContainers = val
	return run
}

func (run *Run) SetDisableCache(val bool) *Run {
	run.disableCache = val
	return run
}

func (run *Run) SetContext(ctx context.Context) *Run {
	run.context = ctx
	return run
}

func (run *Run) MountDockerSocket() *Run {
	run.mountDockerSocket = true
	return run
}

func (run *Run) UseDefaultCommand() *Run {
	run.useDefaultCommand = true
	return run
}

func (run *Run) UseDefaultUser() *Run {
	run.useDefaultUser = true
	return run
}

func (run *Run) SetPrivileged(p bool) *Run {
	run.privileged = p
	return run
}

func (run *Run) SetPorts(ports ...string) *Run {
	run.ports = ports
	return run
}

func (run *Run) SetVolumeMounts(volumeMounts ...Volume) *Run {
	run.volumeMounts = volumeMounts
	return run
}

func (run *Run) AddVolumeBinds(volumeBinds ...Volume) *Run {
	for _, v := range volumeBinds {
		run.volumeBinds = append(run.volumeBinds, v)
	}
	return run
}

func (run *Run) SetVolumeBinds(volumeBinds ...Volume) *Run {
	run.volumeBinds = volumeBinds
	return run
}

func (run *Run) Util() *DockerUtil {
	return run.dockerUtil
}

func (run *Run) ContainerID() string {
	return run.containerID
}

func (run *Run) GoContext() context.Context {
	if run.context == nil {
		return context.Background()
	}
	return run.context
}

// calcConfigHash calculates hash sum of configuration (to figure out whether container needs to be re-created)
func (run *Run) calcConfigHash(runCtx RunContext) (string, error) {
	var b bytes.Buffer
	gob.Register(run.volumeBinds)
	err := gob.NewEncoder(&b).Encode([]interface{}{
		run.Reference, run.privileged, run.mountDockerSocket,
		run.envVars, run.entrypoint, run.command,
		run.volumeBinds, run.volumeMounts, run.ports,
		runCtx.User, runCtx.CurrentOS, runCtx.CurrentCI.Name,
		runCtx.Env,
	})
	hash := md5.New()
	hash.Write(b.Bytes())
	return hex.EncodeToString(hash.Sum(nil)), err
}

// RunContext helpers
func svcCommandDetached(runCtx RunContext, command string) ExecContext {
	clone := runCtx.Clone()
	writer := ioutils.NopWriteCloser(&ioutils.NopWriter{})
	clone.Stdin = nil
	clone.Stderr = writer
	return ExecContext{
		serviceCmd:        true,
		runCtx:            &clone,
		command:           command,
		doNotAttachStdIn:  true,
		doNotAttachStdOut: true,
		detach:            true,
	}
}

func svcCommand(runCtx RunContext, command string) ExecContext {
	return ExecContext{
		serviceCmd:        true,
		runCtx:            &runCtx,
		command:           command,
		doNotAttachStdIn:  true,
		doNotAttachStdOut: true,
	}
}

func (ctx *ExecContext) String() string {
	return fmt.Sprintf("as user '%s', exec (service=%t): '%s'", ctx.runCtx.User, ctx.serviceCmd, ctx.command)
}

func (ctx *RunContext) isDebug() bool {
	return !ctx.Silent && ctx.Debug
}

func (ctx *RunContext) OS() string {
	if ctx.CurrentOS != "" {
		return ctx.CurrentOS
	}
	return runtime.GOOS
}

func (ctx *RunContext) isLinux() bool {
	return ctx.OS() == `linux`
}

func (ctx *RunContext) isMacOS() bool {
	return ctx.OS() == `darwin`
}

func (ctx *RunContext) logger(onlyDebug bool) util.Logger {
	if onlyDebug && ctx.isDebug() {
		if ctx.Logger != nil {
			return ctx.Logger
		}
		return util.NewStdoutLogger(ctx.Stdout, ctx.Stderr)
	}
	return &util.NoopLogger{}
}

// HostnameOfHost returns host name to access host system
func (ctx *RunContext) HostnameOfHost() string {
	if ctx.isLinux() {
		return GatewayHostname
	}
	return HostSystemHostname
}

// CloneAs clones context as user with workdir
func (ctx *RunContext) CloneAs(username string, workDir string) RunContext {
	otherCtx := ctx.Clone()
	otherCtx.User = username
	otherCtx.WorkDir = workDir
	return otherCtx
}

func (ctx *RunContext) VerboseFlag(flag string) string {
	if ctx.Debug {
		return flag
	}
	return ""
}

// Clone returns copy of context
func (ctx *RunContext) Clone() RunContext {
	res := RunContext{
		User:            ctx.User,
		Stdout:          ctx.Stdout,
		Stderr:          ctx.Stderr,
		Stdin:           ctx.Stdin,
		Prefix:          ctx.Prefix,
		WorkDir:         ctx.WorkDir,
		Silent:          ctx.Silent,
		Debug:           ctx.Debug,
		CurrentOS:       ctx.CurrentOS,
		CurrentCI:       ctx.CurrentCI,
		Tty:             ctx.Tty,
		ErrorOnExitCode: ctx.ErrorOnExitCode,
		Logger:          ctx.Logger,
		Env:             make([]string, len(ctx.Env)),
		RunBeforeExec:   ctx.RunBeforeExec,
		RunAfterExec:    ctx.RunAfterExec,
	}
	copy(res.Env, ctx.Env)
	return res
}

func (ctx *RunContext) Debugf(msgfmt string, args ...interface{}) {
	ctx.logger(true).Debugf(msgfmt, args...)
}

func (ctx *RunContext) Warnf(msgfmt string, args ...interface{}) {
	ctx.logger(false).Log(color.RedString("WARN: ") + color.YellowString(fmt.Sprintf(msgfmt), args...))
}

// cmdSuffix returns suffix for each service command needed to be executed in the container
// empty if there's a need to debug output, otherwise sends output into /dev/null
func (ctx *RunContext) cmdSuffix() string {
	if ctx.isDebug() {
		return ""
	}
	return "> /dev/null 2>&1"
}

//
// Volume helpers
//

// NameOrPathToName returns volume name if specified or host path converted to name with prefix
func (v Volume) NameOrPathToName(prefix string) string {
	if v.Name != "" {
		return v.Name
	}
	return fmt.Sprintf("%s%s", prefix, util.Hash(v.HostPath))
}

// IsRW returns true if volume definition is in read-write mode
func (m VolumeMode) IsRW() bool {
	return m != VolumeModeRO
}

func (m VolumeMode) ConsistencyMode() string {
	if m.IsRW() && m != VolumeModeRW {
		return string(m)
	}
	return string(VolumeModeDefault)
}

//
// Dockerfile helpers
//

// GoContext returns context if specified, otherwise returns background
func (dockerFile *Dockerfile) GoContext() context.Context {
	if dockerFile.Context != nil {
		return dockerFile.Context
	}
	return context.Background()
}

// ID returns ID of the built image (is set after dockerFile.Build())
func (dockerFile *Dockerfile) ID() string {
	return dockerFile.id
}

func (m *ResponseMessage) Summary() string {
	return m.summary
}

// MessageToLogFunc returns callback function that adds prefix of a certain subject to each line and logs it to the logger
func MessageToLogFunc(logger util.Logger, subject string) MsgCallback {
	return func(message *ResponseMessage, err error) {
		prefix := ""
		if err != nil {
			prefix = "ERROR: "
		}
		procFunc := util.ReaderToLogFunc(bytes.NewBufferString(message.Summary()), err != nil, prefix, logger, subject)
		if err := procFunc(); err != nil {
			logger.Logf("ERROR: failed to stream Docker log messages: ", err)
			return
		}
	}
}
