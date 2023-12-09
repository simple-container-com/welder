package types

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"os"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"syscall"

	"github.com/fatih/color"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/git"
	"github.com/simple-container-com/welder/pkg/util"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"gopkg.in/yaml.v2"
)

const DefaultProjectName = "welder-project"

type (
	StringValue string
	BuildArgs   map[string]StringValue
	BuildEnv    map[string]StringValue
)

type (
	VolumesDefinition    []string
	ProfilesDefinition   map[string]ProfileDefinition
	ModulesDefinition    []ModuleDefinition
	TasksDefinition      map[string]TaskDefinition
	BuildMode            string
	DeployEnvsDefinition map[string]DeployEnvDefinition
)

func (environments *DeployEnvsDefinition) AddIfNotExist(envName string, env DeployEnvDefinition) {
	if _, ok := (*environments)[envName]; !ok {
		(*environments)[envName] = env
	}
}

type SyncMode string

const (
	SyncModeBind     SyncMode = "bind"     // create bind volume (default)
	SyncModeCopy     SyncMode = "copy"     // use copy with Docker copy
	SyncModeExternal SyncMode = "external" // sync with external tool (e.g. mutagen)
	SyncModeVolume   SyncMode = "volume"   // use pre-created volumes
	SyncModeAdd      SyncMode = "add"      // add to build container using Docker build context
)

type CommonCtx struct {
	version string
	logger  util.Logger

	Parallel         bool
	ParallelCount    int
	Modules          []string
	SoxEnabled       bool
	SkipTestsEnabled bool
	SimulateOS       string
	CurrentCI        util.CurrentCI
	NoCache          bool
	SyncMode         SyncMode
	ReuseContainers  bool
	RemoveOrphans    bool
	ForceOnHost      bool
	DockerImages     []string
	Profiles         []string
	BuildArgs        BuildArgs
	Username         string
	Verbose          bool
	Strict           bool

	parallelEg             *errgroup.Group
	parallelSem            *semaphore.Weighted
	context                context.Context
	cancelFunc             context.CancelFunc
	cancelled              *atomic.Bool
	rootDir                string
	dockerImage            *OutDockerImageDefinition
	subResolveContextDepth *sync.Map // depth of resolving expressions or conditions that use each other
	lastExecOutput         string    // last execution output
	executingTasks         *sync.Map // currently executing task(s)
	gitClient              git.Git
}

// CancelOnSignal calls Cancel when interruption signal is caught
func (commonCtx *CommonCtx) CancelOnSignal() {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGKILL)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGINT)

	go func() {
		sig := <-signalCh
		commonCtx.Logger().Debugf("caught signal: %+v", sig)
		commonCtx.Cancel("signal %+v", sig)
	}()
}

// IsCancelled returns true if context was cancelled
func (commonCtx *CommonCtx) IsCancelled() bool {
	return commonCtx.cancelled.Load()
}

// Cancel cancels build context
func (commonCtx *CommonCtx) Cancel(fmtReason string, args ...interface{}) {
	commonCtx.cancelled.Store(true)
	commonCtx.Logger().Logf(color.YellowString("WARN: interrupting build context due to: "+fmtReason), args...)
	commonCtx.cancelFunc()
}

func (commonCtx *CommonCtx) IncrementSubResolveContextDepth(name string) {
	if val, ok := commonCtx.subResolveContextDepth.Load(name); !ok {
		commonCtx.subResolveContextDepth.Store(name, 1)
	} else {
		commonCtx.subResolveContextDepth.Store(name, val.(int)+1)
	}
}

func (commonCtx *CommonCtx) DecrementSubResolveContextDepth(name string) {
	if val, ok := commonCtx.subResolveContextDepth.Load(name); ok {
		commonCtx.subResolveContextDepth.Store(name, val.(int)-1)
	}
}

func (commonCtx *CommonCtx) SubResolveContextDepth(name string) int {
	if val, ok := commonCtx.subResolveContextDepth.Load(name); ok {
		return val.(int)
	}
	return 0
}

func (commonCtx *CommonCtx) SetLastExecOutput(output string) {
	commonCtx.lastExecOutput = output
}

func (commonCtx *CommonCtx) LastExecOutput() string {
	return commonCtx.lastExecOutput
}

func (commonCtx *CommonCtx) IsExecutingTask(task string) bool {
	if val, ok := commonCtx.executingTasks.Load(task); ok {
		return val.(bool)
	}
	return false
}

func (commonCtx *CommonCtx) ExecutingTask(name string) {
	commonCtx.executingTasks.Store(name, true)
}

func (commonCtx *CommonCtx) ExecutedTask(name string) {
	commonCtx.executingTasks.Store(name, false)
}

func (commonCtx *CommonCtx) SetVersion(version string) {
	commonCtx.version = version
}

func (commonCtx *CommonCtx) Version() string {
	return commonCtx.version
}

func (commonCtx *CommonCtx) SetRootDir(path string) {
	commonCtx.rootDir = path
}

func (commonCtx *CommonCtx) RootDir() string {
	return commonCtx.rootDir
}

func (commonCtx *CommonCtx) OS() string {
	if commonCtx.SimulateOS != "" {
		return commonCtx.SimulateOS
	}
	return runtime.GOOS
}

func (commonCtx *CommonCtx) CI() util.CurrentCI {
	return commonCtx.CurrentCI
}

func (commonCtx *CommonCtx) IsRunningInBamboo() bool {
	return commonCtx.CurrentCI.IsRunningInBamboo()
}

func (commonCtx *CommonCtx) IsRunningInBitbucketPipelines() bool {
	return commonCtx.CurrentCI.IsRunningInBitbucketPipelines()
}

func (commonCtx *CommonCtx) IsRunningInCI() bool {
	return commonCtx.IsRunningInBamboo() || commonCtx.IsRunningInBitbucketPipelines()
}

func (commonCtx *CommonCtx) GoContext() context.Context {
	if commonCtx.context != nil {
		return commonCtx.context
	}
	return context.Background()
}

func (commonCtx *CommonCtx) SetCurrentDockerImage(o *OutDockerImageDefinition) {
	commonCtx.dockerImage = o
}

func (commonCtx *CommonCtx) CurrentDockerImage() *OutDockerImageDefinition {
	return commonCtx.dockerImage
}

// CalcHash calculates hash sum of configuration
func (commonCtx *CommonCtx) CalcHash() (string, error) {
	var b bytes.Buffer
	subResolveValues := make(map[string]bool, 0)
	if commonCtx.subResolveContextDepth != nil {
		commonCtx.subResolveContextDepth.Range(func(key, value interface{}) bool {
			subResolveValues[key.(string)] = value.(int) > 0
			return true
		})
	}
	executingTasks := make(map[string]bool, 0)
	if commonCtx.executingTasks != nil {
		commonCtx.executingTasks.Range(func(key, value interface{}) bool {
			executingTasks[key.(string)] = value.(bool)
			return true
		})
	}
	gob.Register(executingTasks)
	gob.Register(subResolveValues)
	gob.Register(commonCtx)
	values := []interface{}{commonCtx, subResolveValues, commonCtx.rootDir, executingTasks}
	if commonCtx.dockerImage != nil {
		gob.Register(commonCtx.dockerImage)
		values = append(values, commonCtx.dockerImage)
	}
	err := gob.NewEncoder(&b).Encode(values)
	return base64.StdEncoding.EncodeToString(b.Bytes()), err
}

const RootBuildDefinitionSchemaVersion = "1.8.1"

type VersionedDefinition struct {
	SchemaVersion string `yaml:"schemaVersion,omitempty" json:"schemaVersion,omitempty" jsonschema:"title=Schema version,description=Version of the schema,default=1.5.0"`
}

type RootBuildDefinition struct {
	VersionedDefinition `yaml:",inline"`
	Version             string             `yaml:"version,omitempty" json:"version,omitempty" jsonschema:"title=Version of the project,example=1.0.0,required=false"`
	ProjectName         string             `yaml:"projectName,omitempty" json:"projectName,omitempty" jsonschema:"title=Name of the project,example=my-super-project"`
	ProjectRoot         string             `yaml:"projectRoot,omitempty" json:"projectRoot,omitempty" jsonschema:"title=Root directory for the project ,default=."`
	Default             DefaultDefinition  `yaml:"default,omitempty" json:"default,omitempty"`
	Profiles            ProfilesDefinition `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Modules             ModulesDefinition  `yaml:"modules,omitempty" json:"modules,omitempty"`
	Tasks               TasksDefinition    `yaml:"tasks,omitempty" json:"tasks,omitempty"`

	rootDir               string
	actualBuildDefsCache  sync.Map
	actualDeployDefsCache sync.Map
	actualDockerDefsCache sync.Map
	actualTaskDefsCache   sync.Map
}

func (root *RootBuildDefinition) initCaches() {
	// root.actualTaskDefsCache = make(map[string]TaskDefinition)
	// root.actualDockerDefsCache = make(map[string][]DockerImageDefinition)
	// root.actualDeployDefsCache = make(map[string]DeployDefinition)
	// root.actualBuildDefsCache = make(map[string]BuildDefinition)
}

func (root *RootBuildDefinition) CacheDockerDef(cacheKey string, images []DockerImageDefinition) {
	root.actualDockerDefsCache.Store(cacheKey, images)
}

func (root *RootBuildDefinition) GetCachedDockerDef(cacheKey string) ([]DockerImageDefinition, bool) {
	if res, ok := root.actualDockerDefsCache.Load(cacheKey); ok {
		return res.([]DockerImageDefinition), ok
	}
	return []DockerImageDefinition{}, false
}

func (root *RootBuildDefinition) CacheTaskDef(cacheKey string, task TaskDefinition) {
	root.actualTaskDefsCache.Store(cacheKey, task)
}

func (root *RootBuildDefinition) GetCachedTaskDef(cacheKey string) (TaskDefinition, bool) {
	if res, ok := root.actualTaskDefsCache.Load(cacheKey); ok {
		return res.(TaskDefinition), ok
	}
	return TaskDefinition{}, false
}

func (root *RootBuildDefinition) CacheBuildDef(cacheKey string, build BuildDefinition) {
	root.actualBuildDefsCache.Store(cacheKey, build)
}

func (root *RootBuildDefinition) GetCachedBuildDef(cacheKey string) (BuildDefinition, bool) {
	if res, ok := root.actualBuildDefsCache.Load(cacheKey); ok {
		return res.(BuildDefinition), ok
	}
	return BuildDefinition{}, false
}

func (root *RootBuildDefinition) CacheDeployDef(cacheKey string, deploy DeployDefinition) {
	root.actualDeployDefsCache.Store(cacheKey, deploy)
}

func (root *RootBuildDefinition) GetCachedDeployDef(cacheKey string) (DeployDefinition, bool) {
	if res, ok := root.actualDeployDefsCache.Load(cacheKey); ok {
		return res.(DeployDefinition), ok
	}
	return DeployDefinition{}, false
}

type DeployEnvDefinition struct {
	AutoDeploy          bool `yaml:"autoDeploy,omitempty" json:"autoDeploy,omitempty" jsonschema:"title=Should this environment be deployed automatically,default=false"`
	CommonRunDefinition `yaml:",inline"`
}

type ModuleDefinition struct {
	Version               string `yaml:"version,omitempty" json:"version,omitempty"`
	Name                  string `yaml:"name,omitempty" json:"name,omitempty"`
	Path                  string `yaml:"path,omitempty" json:"path,omitempty"`
	BasicModuleDefinition `yaml:",inline"`
}

func (md *ModuleDefinition) Clone() (ModuleDefinition, error) {
	clone := ModuleDefinition{}
	b, err := yaml.Marshal(md)
	if err != nil {
		return clone, err
	}
	err = yaml.Unmarshal(b, &clone)
	return clone, err
}

func (td *TaskDefinition) Clone() (TaskDefinition, error) {
	clone := TaskDefinition{}
	b, err := yaml.Marshal(td)
	if err != nil {
		return clone, err
	}
	err = yaml.Unmarshal(b, &clone)
	return clone, err
}

func (sd *StepsDefinition) Clone() (StepsDefinition, error) {
	clone := StepsDefinition{}
	b, err := yaml.Marshal(sd)
	if err != nil {
		return clone, err
	}
	err = yaml.Unmarshal(b, &clone)
	return clone, err
}

func (sd *SimpleStepDefinition) Clone() (SimpleStepDefinition, error) {
	clone := SimpleStepDefinition{}
	b, err := yaml.Marshal(sd)
	if err != nil {
		return clone, err
	}
	err = yaml.Unmarshal(b, &clone)
	return clone, err
}

func (sd *StepsDefinition) ToRunDefinition(definition CommonRunDefinition) CommonRunDefinition {
	return CommonRunDefinition{
		CommonSimpleRunDefinition: sd.CommonSimpleRunDefinition,
		Args:                      definition.Args,
	}
}

type BasicModuleDefinition struct {
	Build        BuildDefinition         `yaml:"build,omitempty" json:"build,omitempty" jsonschema:"title=Build definitions"`
	Deploy       DeployDefinition        `yaml:"deploy,omitempty" json:"deploy,omitempty" jsonschema:"title=Deployment definitions"`
	DockerImages []DockerImageDefinition `yaml:"dockerImages,omitempty" json:"dockerImages,omitempty" jsonschema:"title=List of Docker Images to build"`
}

func (md *BasicModuleDefinition) Clone() (BasicModuleDefinition, error) {
	clone := BasicModuleDefinition{}
	b, err := yaml.Marshal(md)
	if err != nil {
		return clone, err
	}
	err = yaml.Unmarshal(b, &clone)
	return clone, err
}

type DefaultDefinition struct {
	BasicModuleDefinition `yaml:",inline"`
	MainVolume            docker.Volume `yaml:"mainVolume,omitempty" json:"mainVolume,omitempty"`
}

type ProfileDefinition struct {
	BasicModuleDefinition `yaml:",inline"`
	Activation            ProfileActivationDefinition `yaml:"activation,omitempty" json:"activation,omitempty" jsonschema:"title=Automatic profile activation condition"`
}

type ProfileActivationDefinition struct {
	Sox       bool   `yaml:"sox,omitempty" json:"sox,omitempty" jsonschema:"title=Activate profile when running in a compliant environment"`
	SkipTests bool   `yaml:"skip-tests,omitempty" json:"skip-tests,omitempty" jsonschema:"title=Activate profile when skip-tests mode is enabled"`
	Bamboo    bool   `yaml:"bamboo,omitempty" json:"bamboo,omitempty" jsonschema:"title=Activate profile when running on Bamboo"`
	Verbose   bool   `yaml:"verbose,omitempty" json:"verbose,omitempty" jsonschema:"title=Activate profile when verbose mode is enabled"`
	Strict    bool   `yaml:"strict,omitempty" json:"strict,omitempty" jsonschema:"title=Activate profile when strict mode is enabled"`
	Parallel  bool   `yaml:"parallel,omitempty" json:"parallel,omitempty" jsonschema:"title=Activate profile when parallel mode is enabled"`
	Pipelines bool   `yaml:"pipelines,omitempty" json:"pipelines,omitempty" jsonschema:"title=Activate profile when running on Bitbucket Pipelines"`
	Linux     bool   `yaml:"linux,omitempty" json:"linux,omitempty" jsonschema:"title=Activate profile when running on Linux"`
	Darwin    bool   `yaml:"darwin,omitempty" json:"darwin,omitempty" jsonschema:"title=Activate profile when running on MacOS"`
	Windows   bool   `yaml:"windows,omitempty" json:"windows,omitempty" jsonschema:"title=Activate profile when running on Windows"`
	If        string `yaml:"if,omitempty" json:"if,omitempty" jsonschema:"title=Activate profile when condition is true"`
}

type CommonSimpleRunDefinition struct {
	Env              BuildEnv          `yaml:"env,omitempty" json:"env,omitempty" jsonschema:"title=Environment variables"`
	Volumes          VolumesDefinition `yaml:"volumes,omitempty" json:"volumes,omitempty" jsonschema:"title=Volumes to mount to container"`
	WorkDir          string            `yaml:"workDir,omitempty" json:"workDir,omitempty" jsonschema:"title=Working directory (module dir by default)"`
	ContainerWorkDir string            `yaml:"containerWorkDir,omitempty" json:"containerWorkDir,omitempty" jsonschema:"title=Working directory within container (same as host dir by default)"`
	InjectEnv        []string          `yaml:"injectEnv,omitempty" json:"injectEnv,omitempty" jsonschema:"title=Wildcard patterns of host env variables to pass into container,example=*_TAG"`
}

type CommonRunDefinition struct {
	CommonSimpleRunDefinition `yaml:",inline"`
	Args                      BuildArgs `yaml:"args,omitempty" json:"args,omitempty" jsonschema:"title=Additional arguments to pass to commands"`
}

type SimpleStepDefinition struct {
	Image   string    `yaml:"image,omitempty" json:"image,omitempty" jsonschema:"title=Docker image to use when running in container,oneof_required=image"`
	Scripts []string  `yaml:"script,omitempty" json:"script,omitempty" jsonschema:"title=Commands to execute"`
	RunOn   RunOnType `yaml:"runOn,omitempty" json:"runOn,omitempty" jsonschema:"enum=container,enum=host,title=Run mode (container || host),default=container,oneof_required=runOn"`
	RunIf   string    `yaml:"runIf,omitempty" json:"runIf,omitempty" jsonschema:"title=Condition to execute step,example=${mode:bitbucket}"`
}

type RunAfterStepDefinition struct {
	SimpleStepDefinition `yaml:",inline"`
	Tasks                []string `yaml:"tasks,omitempty" json:"tasks,omitempty" jsonschema:"title=Names of the tasks to invoke,oneof_required=tasks"`
}

type StepsDefinition struct {
	CommonSimpleRunDefinition `yaml:",inline"`
	Name                      string         `yaml:"name,omitempty" json:"name,omitempty" jsonschema:"title=Name of the step to execute"`
	Step                      StepDefinition `yaml:"step,omitempty" json:"step,omitempty" jsonschema:"title=Definition of the step,oneof_required=step"`
	Task                      string         `yaml:"task,omitempty" json:"task,omitempty" jsonschema:"title=Name of the task to invoke,oneof_required=task"`
	Pipe                      string         `yaml:"pipe,omitempty" json:"pipe,omitempty" jsonschema:"title=Bitbucket Pipelines pipe to invoke,oneof_required=pipe"`
}

type StepDefinition struct {
	SimpleStepDefinition `yaml:",inline"`
	CustomImage          DockerImageDefinition `yaml:"customImage,omitempty" json:"customImage,omitempty" jsonschema:"title=Custom Docker image definition,oneof_required=customImage"`
}

type TaskDefinition struct {
	CommonRunDefinition `yaml:",inline"`
	StepDefinition      `yaml:",inline"`
	Description         string `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"title=Description of the task"`
}

type BuildDefinition struct {
	CommonRunDefinition `yaml:",inline"`
	Steps               []StepsDefinition `yaml:"steps,omitempty" json:"steps,omitempty" jsonschema:"title=Steps to execute within the build"`
}

type DeployDefinition struct {
	BuildDefinition `yaml:",inline"`
	Environments    DeployEnvsDefinition `yaml:"environments,omitempty" json:"environments,omitempty" jsonschema:"title=Deployment environments"`
}

func (rd *CommonRunDefinition) ActualContainerWorkDir(root RootBuildDefinition) (string, error) {
	// Use current directory path by default (workaround volumes when Docker-in-Docker is used)
	if rd.ContainerWorkDir != "" {
		return rd.ContainerWorkDir, nil
	}
	return path.Join(root.ProjectRoot, rd.WorkDir), nil
}

func (sd *SimpleStepDefinition) ToRunSpec(name string, run CommonRunDefinition) RunSpec {
	return RunSpec{
		Name:    name,
		Image:   sd.Image,
		Scripts: sd.Scripts,
		RunOn:   sd.RunOn,
		RunCfg:  run,
		RunIf:   sd.RunIf,
	}
}

func (sd *StepDefinition) ToRunSpec(name string, run CommonRunDefinition) (res RunSpec) {
	res = sd.SimpleStepDefinition.ToRunSpec(name, run)
	res.CustomImage = sd.CustomImage
	return
}

func (td *TaskDefinition) ToRunSpec(name string) RunSpec {
	return td.StepDefinition.ToRunSpec(name, td.CommonRunDefinition)
}

type RunOnType string

// IsContainer returns true if task should run in container (default)
func (rt RunOnType) IsContainer() bool {
	return rt != RunOnTypeHost
}

func (RunOnType) Enum() []interface{} {
	return []interface{}{
		RunOnTypeHost,
		RunOnTypeContainer,
	}
}

const (
	RunOnTypeHost      RunOnType = "host"
	RunOnTypeContainer RunOnType = "container"
)

type RunSpec struct {
	RunCfg      CommonRunDefinition
	CustomImage DockerImageDefinition
	Name        string
	Image       string
	RunOn       RunOnType
	RunIf       string
	Scripts     []string
}

const OutDockerSchemaVersion = "1.0"

type OutDockerDefinition struct {
	SchemaVersion string                      `yaml:"schemaVersion,omitempty" json:"schemaVersion,omitempty" jsonschema:"title=Version of Welder tool"`
	Modules       []OutDockerModuleDefinition `yaml:"modules,omitempty" json:"modules,omitempty"`
}

type OutDockerModuleDefinition struct {
	Name         string                     `yaml:"name,omitempty" json:"name,omitempty"`
	DockerImages []OutDockerImageDefinition `yaml:"dockerImages,omitempty" json:"dockerImages,omitempty"`
}

type OutDockerImageDefinition struct {
	Name    string                      `yaml:"name,omitempty" json:"name,omitempty"`
	Digests []OutDockerDigestDefinition `yaml:"digests,omitempty" json:"digests,omitempty"`
}

type OutDockerDigestDefinition struct {
	Tag    string `yaml:"tag,omitempty" json:"tag,omitempty"`
	Image  string `yaml:"image,omitempty" json:"image,omitempty"`
	Digest string `yaml:"digest,omitempty" json:"digest,omitempty"`
}

type DockerImageDefinition struct {
	Name             string                 `yaml:"name,omitempty" json:"name,omitempty" jsonschema:"title=Name of the Docker image to build"`
	DockerFile       string                 `yaml:"dockerFile,omitempty" json:"dockerFile,omitempty" jsonschema:"title=Dockerfile to use with the docker build,oneof_required=dockerfile"`
	Tags             []string               `yaml:"tags,omitempty" json:"tags,omitempty" jsonschema:"title=Tags to apply to the built Docker image"`
	Build            DockerBuildDefinition  `yaml:"build,omitempty" json:"build,omitempty" jsonschema:"title=Build definition of the Docker image"`
	InlineDockerfile string                 `yaml:"inlineDockerFile,omitempty" json:"inlineDockerFile,omitempty" jsonschema:"title=Inline text of the Dockerfile to build,oneof_required=inlinedockerfile"`
	RunAfterBuild    RunAfterStepDefinition `yaml:"runAfterBuild,omitempty" json:"runAfterBuild,omitempty" jsonschema:"title=Step to run after Docker image is built"`
	RunAfterPush     RunAfterStepDefinition `yaml:"runAfterPush,omitempty" json:"runAfterPush,omitempty" jsonschema:"title=Step to run after Docker image is pushed"`
}

// IsValid returns true if docker image definition is valid
func (d DockerImageDefinition) IsValid() bool {
	return d.DockerFile != "" || d.InlineDockerfile != ""
}

type DockerBuildDefinition struct {
	ContextPath string           `yaml:"contextPath,omitempty" json:"contextPath,omitempty" jsonschema:"title=Context path for the Docker build (Docker context)"`
	Args        []DockerBuildArg `yaml:"args,omitempty" json:"args,omitempty" jsonschema:"title=Build arguments for the Docker build"`
}

type DockerBuildArg struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty" jsonschema:"title=Name of the argument to pass to Docker build"`
	Value string `yaml:"value,omitempty" json:"value,omitempty" jsonschema:"title=Value of the argument to pass to Docker build"`
	File  string `yaml:"file,omitempty" json:"file,omitempty" jsonschema:"title=Pass file contents to Docker build as a single value"`
}
