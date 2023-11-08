package types

import (
	"context"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/git"
	"github.com/smecsia/welder/pkg/util"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	BuildModeSox = "sox"
)

// RawModuleConfig returns module configuration without any processing
func (root *RootBuildDefinition) RawModuleConfig(moduleName string) (ModuleDefinition, error) {
	for _, module := range root.Modules {
		if module.Name == moduleName {
			return module.Clone()
		}
	}
	return ModuleDefinition{}, fmt.Errorf("module not found: %s", moduleName)
}

// RootDirPath returns root path of the project (where root descriptor is located - i.e. not ${project:root})
func (root *RootBuildDefinition) RootDirPath() string {
	return root.rootDir
}

// ConfiguredRootPath returns root path of the project (either configured or default (${project:root})
func (root *RootBuildDefinition) ConfiguredRootPath() string {
	if root.ProjectRoot != "" {
		return root.ProjectRoot
	}
	return root.RootDirPath()
}

// RawTaskConfig returns task configuration without any processing
func (root *RootBuildDefinition) RawTaskConfig(taskName string) (TaskDefinition, error) {
	for name, task := range root.Tasks {
		if name == taskName {
			return task.Clone()
		}
	}
	return TaskDefinition{}, fmt.Errorf("task not found: %s", taskName)
}

// ToDockerVolumes converts string volume definitions into respective structs
func (volumes VolumesDefinition) ToDockerVolumes(commonCtx *CommonCtx) ([]docker.Volume, error) {
	res := make([]docker.Volume, 0)
	for _, v := range volumes {
		parts := strings.SplitN(v, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid volume definition: %s", v)
		}
		mode := ""
		hostPath, targetPath := parts[0], parts[1]
		hostPath, err := homedir.Expand(hostPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to expand host path: %s", hostPath)
		}
		if len(parts) == 3 {
			mode = parts[2]
		}
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			continue
		}
		volumeMode := docker.VolumeMode(mode)

		// hack to force "delegate" volume mode by default when we're not on linux
		// this should speed up builds on MacOS
		if commonCtx.OS() != `linux` && volumeMode == "" {
			volumeMode = docker.VolumeModeDelegated
		}

		res = append(res, docker.Volume{
			HostPath: hostPath,
			ContPath: targetPath,
			Mode:     volumeMode,
		})
	}
	return res, nil
}

// PathTo returns sub path relative to root of the project
func (root *RootBuildDefinition) PathTo(rootDir string, subPath string) string {
	// if path is already absolute, no need to calc relative path
	if path.IsAbs(subPath) {
		return subPath
	}
	rootPath, _, _ := DetectBuildContext(rootDir)
	return filepath.Join(rootPath, subPath)
}

// ActiveModules returns list of active modules names
func (commonCtx *CommonCtx) ActiveModules(root RootBuildDefinition, detectedModule *ModuleDefinition) []string {
	// modules specified by user
	if len(commonCtx.Modules) > 0 {
		return commonCtx.Modules
	}
	// calc modules based on context
	modules := root.ModuleNames()
	if detectedModule != nil {
		modules = []string{detectedModule.Name}
	}
	return modules
}

// Init initializes state of deploy definition
func (deploy *DeployDefinition) Init() *DeployDefinition {
	deploy.BuildDefinition.Init()
	if deploy.Environments == nil {
		deploy.Environments = make(DeployEnvsDefinition)
	}
	for k := range deploy.Environments {
		// god bless non-addressable Go maps
		env := deploy.Environments[k]
		(&env).CommonRunDefinition.Init()
		deploy.Environments[k] = env
	}
	return deploy
}

func (build *BuildDefinition) Init() *BuildDefinition {
	build.CommonRunDefinition.Init()
	return build
}

// Init initializes state of build definition
func (rd *CommonSimpleRunDefinition) Init() *CommonSimpleRunDefinition {
	if rd.Env == nil {
		rd.Env = make(BuildEnv)
	}
	if rd.Volumes == nil {
		rd.Volumes = make(VolumesDefinition, 0)
	}
	if rd.InjectEnv == nil {
		rd.InjectEnv = make([]string, 0)
	}
	return rd
}

// Init initializes state of build definition
func (rd *CommonRunDefinition) Init() *CommonRunDefinition {
	rd.CommonSimpleRunDefinition.Init()
	if rd.Args == nil {
		rd.Args = make(BuildArgs)
	}
	return rd
}

// ArgsToMap reads specified Docker args and converts them to a map consumable by Docker API
func (build *DockerBuildDefinition) ArgsToMap() (map[string]*string, error) {
	res := make(map[string]*string)
	for _, arg := range build.Args {
		if arg.Value != "" {
			val := arg.Value
			res[arg.Name] = &val
		} else if arg.File != "" {
			if path, err := homedir.Expand(arg.File); err != nil {
				return nil, err
			} else if path, err := filepath.Abs(path); err != nil {
				return nil, err
			} else if bytes, err := ioutil.ReadFile(path); err != nil {
				return nil, err
			} else {
				val := string(bytes)
				res[arg.Name] = &val
			}
		}
	}
	return res, nil
}

// IsSox returns true if sox mode was enabled
func (commonCtx *CommonCtx) IsSox() bool {
	return commonCtx.BuildMode() == BuildModeSox
}

// BuildMode returns "sox" if sox mode was enabled
func (commonCtx *CommonCtx) BuildMode() BuildMode {
	if commonCtx.SoxEnabled {
		return BuildModeSox
	}
	return ""
}

// ModuleNames returns list of defines module names
func (root *RootBuildDefinition) ModuleNames() []string {
	names := make([]string, len(root.Modules))
	for i, module := range root.Modules {
		names[i] = module.Name
	}
	return names
}

// ProfileNames returns list of defines profile names
func (root *RootBuildDefinition) ProfileNames() []string {
	var names []string
	for name := range root.Profiles {
		names = append(names, name)
	}
	return names
}

// ToBuildEnv converts environment variables to array of strings
func (envs *BuildEnv) ToBuildEnv(localEnvFilters ...*regexp.Regexp) []string {
	res := make([]string, len(*envs))
	var i int
	for k, v := range *envs {
		res[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	for _, keyVal := range os.Environ() {
		parts := strings.SplitN(keyVal, "=", 2)
		for _, filter := range localEnvFilters {
			if filter.Match([]byte(parts[0])) {
				res = append(res, keyVal)
			}
		}
	}
	return res
}

// ParseBuildEnv converts array of strings to BuildEnv
func ParseBuildEnv(env []string) BuildEnv {
	res := make(BuildEnv)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			res[parts[0]] = StringValue(parts[1])
		}
	}
	return res
}

func (args BuildArgs) Copy() BuildArgs {
	cp := make(BuildArgs)
	for k, v := range args {
		cp[k] = v
	}
	return cp
}

// UnmarshalYAML fix marshalling
func (envs *BuildEnv) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type BuildEnv_ BuildEnv // prevent recursion
	hashed := make(BuildEnv_)
	if err := unmarshal(&hashed); err != nil {
		return unmarshal(&hashed)
	}
	*envs = BuildEnv(hashed)
	return nil
}

// MarshalYAML fix marshalling
func (val StringValue) MarshalYAML() (interface{}, error) {
	res := fmt.Sprintf("\n%s\n", string(val))
	return res, nil
}

// UnmarshalYAML fix marshalling
func (val *StringValue) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type StringValue_ StringValue // prevent recursion
	var unmarshVal StringValue_
	if err := unmarshal(&unmarshVal); err != nil {
		return err
	}
	*val = StringValue(strings.Trim(string(unmarshVal), "\n"))
	return nil
}

// ActualVersion returns actual version defined for module or root version by default
func (md ModuleDefinition) ActualVersion(root RootBuildDefinition) (string, error) {
	if md.Version == "" && root.Version != "" {
		return root.Version, nil
	} else if md.Version != "" {
		return md.Version, nil
	}
	return "", errors.Errorf("version is empty for module %s", md.Name)
}

// InjectEnvRegex returns regexps
func (rd CommonRunDefinition) InjectEnvRegex(commonCtx *CommonCtx) []*regexp.Regexp {
	res := make([]*regexp.Regexp, 0)
	for _, expr := range rd.InjectEnv {
		if rgx, err := regexp.Compile(expr); err == nil {
			res = append(res, rgx)
		} else {
			commonCtx.Logger().Logf("WARN: failed to compile pattern '%s'", expr, err)
		}
	}
	return res
}

// NewCommonContext initializes new common context (a copy of the one provided)
func NewCommonContext(ctx *CommonCtx, logger util.Logger) *CommonCtx {
	var goCtx = ctx.context
	var parallelEg = ctx.parallelEg
	var cancelFunc = ctx.cancelFunc
	if ctx.parallelEg == nil || ctx.cancelFunc == nil || ctx.context == nil {
		parallelEg, goCtx = errgroup.WithContext(context.Background())
		goCtx, cancelFunc = context.WithCancel(goCtx)
	}
	if ctx.subResolveContextDepth == nil {
		ctx.subResolveContextDepth = &sync.Map{}
	}
	if ctx.executingTasks == nil {
		ctx.executingTasks = &sync.Map{}
	}
	newCommonCtx := CommonCtx{
		Parallel:               ctx.Parallel,
		ParallelCount:          ctx.ParallelCount,
		SkipTestsEnabled:       ctx.SkipTestsEnabled,
		SoxEnabled:             ctx.SoxEnabled,
		Modules:                make([]string, len(ctx.Modules)),
		SimulateOS:             ctx.SimulateOS,
		CurrentCI:              ctx.CurrentCI,
		NoCache:                ctx.NoCache,
		SyncMode:               ctx.SyncMode,
		ReuseContainers:        ctx.ReuseContainers,
		RemoveOrphans:          ctx.RemoveOrphans,
		ForceOnHost:            ctx.ForceOnHost,
		Username:               ctx.Username,
		Verbose:                ctx.Verbose,
		Strict:                 ctx.Strict,
		BuildArgs:              ctx.BuildArgs.Copy(),
		Profiles:               make([]string, len(ctx.Profiles)),
		context:                goCtx,
		cancelFunc:             cancelFunc,
		parallelEg:             parallelEg,
		cancelled:              atomic.NewBool(false),
		parallelSem:            semaphore.NewWeighted(int64(ctx.ParallelCount)),
		logger:                 logger,
		version:                ctx.version,
		rootDir:                ctx.rootDir,
		dockerImage:            ctx.dockerImage,
		subResolveContextDepth: ctx.subResolveContextDepth,
		lastExecOutput:         ctx.lastExecOutput,
		executingTasks:         ctx.executingTasks,
		gitClient:              ctx.gitClient,
	}
	copy(newCommonCtx.Modules, ctx.Modules)
	return &newCommonCtx
}

// SetGitClient overwrites default git client
func (commonCtx *CommonCtx) SetGitClient(gitClient git.Git) {
	commonCtx.gitClient = gitClient
}

func (commonCtx *CommonCtx) GitClient() git.Git {
	return commonCtx.gitClient
}

// InitGitClientIfNeeded intilaizes git client if needed
func (commonCtx *CommonCtx) InitGitClientIfNeeded() {
	if commonCtx.gitClient == nil {
		gitClient, err := git.TraverseToRoot()
		if err != nil {
			commonCtx.logger.Debugf("failed to detect Git root: %s", err.Error())
		}
		commonCtx.gitClient = gitClient
	}
}

// Logger returns logger or default to Stdout
func (commonCtx *CommonCtx) Logger() util.Logger {
	if commonCtx.logger == nil {
		return util.NewStdoutLogger(os.Stdout, os.Stderr)
	}
	return commonCtx.logger
}

// ProjectNameOrDefault returns project name or default value if empty
func (root *RootBuildDefinition) ProjectNameOrDefault() string {
	if root.ProjectName != "" {
		return root.ProjectName
	}
	return DefaultProjectName
}

// StartParallel starts some job synchronously or asynchronously (depending on parallel config)
// it also respects ParallelCount parameter
func (commonCtx *CommonCtx) StartParallel(callback func() error) error {
	// if no parallel execution requested
	if !commonCtx.Parallel {
		return callback()
	}
	// if need to execute concurrently
	commonCtx.parallelEg.Go(func() error {
		// if parallel count has a limit, block on semaphore
		if commonCtx.ParallelCount > 0 {
			err := commonCtx.parallelSem.Acquire(commonCtx.GoContext(), 1)
			if err != nil {
				return err
			}
			defer util.SafeReleaseSemaphore(commonCtx.parallelSem, 1, nil)()
		}
		// if cancelled, do not start executing
		if commonCtx.IsCancelled() {
			return errors.Errorf("context was cancelled")
		}
		return callback()
	})
	return nil
}

// WaitParallel waits until all parallel execution finish
func (commonCtx *CommonCtx) WaitParallel() error {
	return commonCtx.parallelEg.Wait()
}

func MergeMapIfEmpty(source map[string]StringValue, target map[string]StringValue) {
	for srcKey, srcVal := range source {
		if val, ok := target[srcKey]; !ok || val == "" {
			target[srcKey] = srcVal
		}
	}
}

func OverrideMapValuesIfNotEmpty(source map[string]StringValue, target map[string]StringValue) {
	for srcKey, srcVal := range source {
		if srcVal != "" {
			target[srcKey] = srcVal
		}
	}
}

func AppendToListIfNotExist(target []string, values []string) []string {
	for _, value := range values {
		found := false
		for _, existVal := range target {
			if existVal == value {
				found = true
				break
			}
		}
		if !found {
			target = append(target, value)
		}
	}
	return target
}

func MergeRunDefinitions(from CommonRunDefinition, to *CommonRunDefinition, override bool) {
	MergeSimpleRunDefinitions(from.CommonSimpleRunDefinition, &to.CommonSimpleRunDefinition, override)
	if to.Args == nil {
		to.Args = make(BuildArgs)
	}
	MergeMapIfEmpty(from.Args, to.Args)
}

func MergeSimpleRunDefinitions(from CommonSimpleRunDefinition, to *CommonSimpleRunDefinition, override bool) {
	to.Volumes = AppendToListIfNotExist(to.Volumes, from.Volumes)
	to.InjectEnv = AppendToListIfNotExist(to.InjectEnv, from.InjectEnv)
	if override || (to.ContainerWorkDir == "" && from.ContainerWorkDir != "") {
		to.ContainerWorkDir = from.ContainerWorkDir
	}
	if override || (to.WorkDir == "" && from.WorkDir != "") {
		to.WorkDir = from.WorkDir
	}
	if to.Env == nil {
		to.Env = make(BuildEnv)
	}
	MergeMapIfEmpty(from.Env, to.Env)

}

// ActualStepsDefinitionFor builds effective steps definition for a step of a build
func (root *RootBuildDefinition) ActualStepsDefinitionFor(build *BuildDefinition, step *StepsDefinition) (StepsDefinition, error) {
	res, err := step.Clone()
	if err != nil {
		return *step, err
	}
	MergeSimpleRunDefinitions(*build.CommonSimpleRunDefinition.Init(), res.CommonSimpleRunDefinition.Init(), false)
	return res, err
}

func MergeSteps(from BuildDefinition, to *BuildDefinition) error {
	if len(from.Steps) > 0 && len(to.Steps) == 0 {
		clonedSteps, err := cloneSteps(from.Steps)
		if err != nil {
			return err
		}
		to.Steps = clonedSteps
	}
	return nil
}

func cloneSteps(steps []StepsDefinition) ([]StepsDefinition, error) {
	res := make([]StepsDefinition, len(steps))
	for i, step := range steps {
		clone, err := step.Clone()
		if err != nil {
			return res, err
		}
		res[i] = clone
	}
	return res, nil
}
