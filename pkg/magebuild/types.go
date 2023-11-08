package magebuild

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/smecsia/welder/pkg/git"
	"io"
	"strings"
)

type Generator func() error

// Cmd executable command definition
type Cmd struct {
	Command string
	Wd      string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
}

type CommonBuildContext struct {
	configFilePath string
}

type GoBuildContext struct {
	OutDir         string     `yaml:"outDir,omitempty" env:"OUT_DIR" default:"bin"`
	Platforms      []Platform `yaml:"platforms,omitempty"`
	Targets        []Target   `yaml:"targets,omitempty"`
	StatlasURL     string     `yaml:"statlasURL,omitempty" env:"STATLAS_URL" default:"-"`
	StatlasToken   string     `yaml:"statlasToken,omitempty" env:"STATLAS_TOKEN" default:"-"`
	SlauthToken    string     `yaml:"slauthToken,omitempty" env:"SLAUTH_TOKEN" default:"-"`
	BambooJWTToken string     `yaml:"bambooJWTToken,omitempty" env:"bamboo_JWT_TOKEN" default:"-"`

	// env-only fields
	GitAuthor       string `yaml:"-" default:"bambooagent" env:"GIT_AUTHOR"`
	GitBranch       string `yaml:"-" default:"master" env:"GIT_BRANCH"`
	GitRemote       string `yaml:"-" default:"origin" env:"GIT_REMOTE"`
	Parallel        string `yaml:"-" default:"true" env:"PARALLEL"`
	SkipTests       string `yaml:"-" default:"false" env:"SKIP_TESTS"`
	ReleaseChannel  string `yaml:"-" default:"stable" env:"RELEASE_CHANNEL"`
	FilterTargets   string `yaml:"-" default:"-" env:"TARGETS"`
	FilterPlatforms string `yaml:"-" default:"-" env:"PLATFORMS"`
	ExtraBuildFlags string `yaml:"-" default:"-" env:"EXTRA_BUILD_FLAGS"`
	Version         string `yaml:"version,omitempty" env:"VERSION" default:"0.0.1"`
	JUnitOutputFile string `yaml:"junintOutputFile,omitempty" env:"JUNIT_OUTPUT_FILE" default:"bin/junit-report.xml"`

	// init-only private fields
	CommonBuildContext `yaml:",inline"`
	git                git.Git
}

func (ctx *CommonBuildContext) SetConfigFilePath(path string) {
	ctx.configFilePath = path
}

func (ctx *CommonBuildContext) GetConfigFilePath() string {
	return ctx.configFilePath
}

func (ctx *CommonBuildContext) Init() error {
	return nil
}

func (ctx *GoBuildContext) Init() error {
	ctx.git = git.NewWithCfg(ctx.GitRoot(), ctx.GitAuthor, ctx.GitBranch, ctx.GitRemote)
	return ctx.CommonBuildContext.Init()
}

func (ctx *GoBuildContext) GetVersion() string {
	if ctx.Version != "" {
		return ctx.Version
	}
	hash, err := ctx.git.HashShort()
	if err != nil {
		fmt.Println(color.RedString("WARN") + " failed to detect Git hash: " + color.RedString(err.Error()))
	}
	return hash
}

// Platform defines platform to run build for
type Platform struct {
	GOOS       string `yaml:"os,omitempty"`
	GOARCH     string `yaml:"arch,omitempty"`
	CgoEnabled bool   `yaml:"cgo,omitempty"`
}

// IsEqualTo returns true if platform matches its string representation
func (p *Platform) String() string {
	cgoSuffix := ""
	if p.CgoEnabled {
		cgoSuffix = "-cgo"
	}
	return fmt.Sprintf("%s-%s%s", p.GOOS, p.GOARCH, cgoSuffix)
}

// IsEqualTo returns true if platform matches its string representation
func (p *Platform) IsEqualTo(platform string) bool {
	return p.String() == platform
}

// Target defines target to run build for
type Target struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path,omitempty"`
}

// Bundle defines result of tarball build
type Bundle struct {
	Target             Target
	Platform           Platform
	TargetChecksumFile string
	TargetBinaryFile   string
	TarFile            string
	TarChecksumFile    string
}

// IsParallel returns true if concurrent execution required
func (ctx *GoBuildContext) IsParallel() bool {
	return ctx.Parallel == "true"
}

// IsSkipTests returns true if tests must be skipped
func (ctx *GoBuildContext) IsSkipTests() bool {
	return ctx.SkipTests == "true"
}

// ActivePlatforms returns list of active platforms
func (ctx *GoBuildContext) ActivePlatforms() []Platform {
	if ctx.FilterPlatforms == "-" {
		return ctx.Platforms
	}
	res := make([]Platform, 0)
	usePlatforms := strings.Split(ctx.FilterPlatforms, ",")
	for _, platform := range ctx.Platforms {
		for _, usePlatform := range usePlatforms {
			if platform.IsEqualTo(usePlatform) {
				res = append(res, platform)
			}
		}
	}
	fmt.Println("Active platforms: ", res)
	return res
}

// ActiveTargets returns true if concurrent execution required
func (ctx *GoBuildContext) ActiveTargets() []Target {
	if ctx.FilterTargets == "-" {
		return ctx.Targets
	}
	res := make([]Target, 0)
	useTargets := strings.Split(ctx.FilterTargets, ",")
	for _, target := range ctx.Targets {
		for _, useTarget := range useTargets {
			if useTarget == target.Name {
				res = append(res, target)
			}
		}
	}
	fmt.Println("Active targets: ", res)
	return res
}
