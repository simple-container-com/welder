package docker

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/simple-container-com/welder/pkg/docker/dockerext"
	"github.com/simple-container-com/welder/pkg/util"
)

// HomeDockerConfigAuth authentication for .docker/config.json
type HomeDockerConfigAuth struct {
	Auth string `json:"auth,omitempty"`
}

// HomeDockerConfig config.json file spec
type HomeDockerConfig struct {
	Auths map[string]HomeDockerConfigAuth `json:"auths,omitempty"`
}

// IsEmpty returns true if config.json is empty
func (cfg *HomeDockerConfig) IsEmpty() bool {
	return len(cfg.Auths) == 0
}

// TagDigest represents tag pushed to repository
type TagDigest struct {
	Tag    string
	Digest string
	Size   int
}

// Dockerfile represents the source for Docker image building
type Dockerfile struct {
	FilePath               string
	ContextPath            string
	Tags                   []string
	Args                   map[string]*string
	TagDigests             map[string]TagDigest
	DisableNoCache         bool
	ReuseImagesWithSameCfg bool
	DisablePull            bool
	Labels                 map[string]string
	BuilderVersion         string
	DockerIgnoreFile       string
	SkipHashLabel          bool
	id                     string
	client                 dockerext.DockerCLIExt
	Context                context.Context
	rwMutex                *sync.RWMutex
}

// VolumeMode indicates volume write mode
type VolumeMode string

// VolumeApproach defines how to copy volumes (either bind or copy/add)
type VolumeApproach string

const (
	VolumeModeRW         VolumeMode = "rw"
	VolumeModeRO         VolumeMode = "ro"
	VolumeModeDelegated  VolumeMode = "delegated"
	VolumeModeCached     VolumeMode = "cached"
	VolumeModeConsistent VolumeMode = "consistent"
	VolumeModeDefault    VolumeMode = "default"

	// VolumeApproachBind default volume bind approach
	VolumeApproachBind VolumeApproach = "bind"
	// VolumeApproachCopy will force docker cp for each volume instead of creating actual binds (useful when dind)
	VolumeApproachCopy VolumeApproach = "copy"
	// VolumeApproachAdd will force "ADD" in the build container's Dockerfile instead of volumes/binds/copy
	VolumeApproachAdd VolumeApproach = "add"
	// VolumeApproachExternal will skip creation of volumes (assuming they are synced by external tool (e.g. mutagen))
	VolumeApproachExternal VolumeApproach = "external"
)

// Volume defines volume to attach to the container
type Volume struct {
	Name     string     `yaml:"name,omitempty"`
	HostPath string     `yaml:"hostPath,omitempty"`
	ContPath string     `yaml:"contPath,omitempty"`
	Mode     VolumeMode `yaml:"mode,omitempty"`
}

// RunContext defines Docker run configuration
type RunContext struct {
	Prefix          string // logger prefix
	User            string // override username within container
	Stdout          io.WriteCloser
	Stderr          io.WriteCloser
	Stdin           io.ReadCloser
	WorkDir         string                         // work dir inside container
	CurrentOS       string                         // override default OS detected by runtime
	CurrentCI       util.CurrentCI                 // CI detected by runtime or overridden by user
	Silent          bool                           // less output
	Debug           bool                           // output everything (including system actions)
	Tty             bool                           // connect tty when run
	ErrorOnExitCode bool                           // return error if exit code != 0
	Detached        bool                           // run in detached mode (do not destroy after commands finished)
	Env             []string                       // list of environment variables to inject into commands when running
	RunBeforeExec   func(containerID string) error // run this callback before executing commands
	RunAfterExec    func(containerID string) error // run this callback after executing commands
	Logger          util.Logger
}

// ExecContext defines execution context of the specific command
type ExecContext struct {
	runCtx            *RunContext
	command           string // command
	serviceCmd        bool   // true if command is a service command (output is hidden, and command runs as system user)
	ignoreErrors      bool   // ignore errors from commands
	detach            bool   // detach from container after execution
	doNotAttachStdIn  bool   // do not attach stdin to container
	doNotAttachStdOut bool   // do not attach stdout to container
}

// ExecResult represents execution result
type ExecResult struct {
	ExitCode int      // exit code of the command
	Env      []string // environment after execution
	ExecID   string   // execution id from Docker service
	Pid      int      // pid of the executed process
}

// Run defines Docker run command
type Run struct {
	RunID     string // identifier for this Docker Run
	Reference string // base Docker image reference

	volumeBinds       []Volume        // list of volumes to connect in this run
	volumeMounts      []Volume        // list of volumes to connect in this run
	ports             []string        // expose ports spec
	privileged        bool            // request creation of the privileged container
	mountDockerSocket bool            // allow to interact with Docker from inside the created container (will mount docker.sock)
	context           context.Context // go context to rely on
	cleanupOrphans    bool            // remove orphan containers if found before creating new ones
	reuseContainers   bool            // allow reusing existing containers with the same runID (if found)
	disableCache      bool            // if true forces to rebuild build image every time
	volumeApproach    VolumeApproach  // defines how to copy volume binds into container
	envVars           []string        // list of environment variables to inject into the container when creating
	stopTimeout       time.Duration   // how long to wait before killing container
	command           []string        // commands to run in the created container (default: DefaultContainerCommand)
	entrypoint        []string        // entrypoint for the created container
	keepEnvVariables  bool            // if true get env after each executed command and pass to the next one

	dockerAPI         *client.Client
	containerID       string
	network           NetworkData
	dockerUtil        *DockerUtil
	initialConfigHash string
	osDistribution    OSDistribution // detected OS distribution of the base image
	useDefaultCommand bool           // if true, container is using default command specified in the image
	useDefaultUser    bool           // if true, container is using default user specified in the image
}

// NetworkData represents internal info about created container's network
type NetworkData struct {
	ID      string
	Gateway string
}

type ResponseAux struct {
	ID     string `json:"ID"`
	Tag    string `json:"Tag"`
	Digest string `json:"Digest"`
	Size   int    `json:"Size"`
}

// ResponseMessage reflects typical response message from Docker daemon of V1
type ResponseMessage struct {
	Id          string      `json:"id"`
	Status      string      `json:"status"`
	Stream      string      `json:"stream"`
	Aux         ResponseAux `json:"aux"`
	ErrorDetail struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	}
	Progress string `json:"progress"`
	Error    string `json:"error"`

	summary string `json:"-"`
}

// ResponseMessageV2 reflects typical response message from Docker daemon of V2
type ResponseMessageV2 struct {
	Id  string `json:"id"`
	Aux string `json:"aux"` // contains base64-encoded PB object
}
