package welder

import (
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"os"
	"path"
	"strings"
	"time"
)

// Deploy runs deploy steps of the project defined by the build context
func (deployCtx *DeployContext) Deploy() error {
	return deployCtx.forEachModule("deploying project", func(root *types.RootBuildDefinition, modCtx *BuildContext, module string) error {
		return modCtx.RunSteps("deploy", root, module, deployCtx)
	})
}

// Build builds project defined by the build context
func (buildCtx *BuildContext) Build() error {
	return buildCtx.forEachModule("building project", func(root *types.RootBuildDefinition, modCtx *BuildContext, module string) error {
		return modCtx.RunSteps("build", root, module, nil)
	})
}

type forModuleCallback func(*types.RootBuildDefinition, *BuildContext, string) error

func (buildCtx *BuildContext) forEachModule(runDesc string, callback forModuleCallback) error {
	buildStatedAt := time.Now()
	detectedModule, root, err := types.ReadBuildModuleDefinition(buildCtx.RootDir())
	if err != nil {
		return err
	}
	activeModules := buildCtx.ActiveModules(root, detectedModule)
	detectedModuleName := ""
	if detectedModule != nil {
		detectedModuleName = detectedModule.Name
	}
	activeProfiles := buildCtx.ActiveProfiles(&root, detectedModuleName)
	buildCtx.Logger().Logf(" - Starting %s...", runDesc)
	buildCtx.Logger().Logf(" - Build tool version: %q", buildCtx.Version())
	buildCtx.Logger().Logf(" - Active modules: ['%s']", strings.Join(activeModules, "', '"))
	buildCtx.Logger().Logf(" - Active profiles: ['%s']", strings.Join(activeProfiles, "', '"))
	if buildCtx.Parallel {
		buildCtx.Logger().Logf(" - Running in parallel with max: %d", buildCtx.ParallelCount)
	}
	for _, m := range activeModules {
		module := m
		buildCtx := NewBuildContext(buildCtx, buildCtx.Logger().SubLogger(module))
		runFunc := func() error {
			err := callback(&root, buildCtx, module)
			if err != nil {
				buildCtx.Cancel("failed for module %s: %s", module, err.Error())
			}
			return err
		}
		if err := buildCtx.StartParallel(runFunc); err != nil {
			return err
		}
	}
	err = buildCtx.WaitParallel()
	buildCtx.Logger().Logf(" - Finished %s in %s", runDesc, util.FormatDuration(time.Since(buildStatedAt)))
	return err
}

type buildRunContext struct {
	buildCtx *BuildContext
	buildDef types.BuildDefinition
	module   *types.ModuleDefinition
}

type runInContainerParams struct {
	volumes []docker.Volume
	workDir string
}

func (buildCtx *BuildContext) calcModuleBuildRunContext(root *types.RootBuildDefinition, moduleName string, deployCtx *DeployContext) (res *buildRunContext, err error) {
	res = &buildRunContext{
		buildCtx: NewBuildContext(buildCtx, buildCtx.Logger()),
	}
	var module types.ModuleDefinition
	var buildDef types.BuildDefinition
	if deployCtx != nil {
		var deployDef types.DeployDefinition
		deployDef, module, err = res.buildCtx.ActualDeployDefinitionFor(root, moduleName, deployCtx)
		if err != nil {
			return res, err
		}
		buildDef = deployDef.BuildDefinition
	} else {
		buildDef, module, err = res.buildCtx.ActualBuildDefinitionFor(root, moduleName)
	}
	if err != nil {
		err = errors.Wrapf(err, "failed to calculate build definition for module %s", moduleName)
	}
	res.buildDef = buildDef
	res.module = &module
	return
}

func (buildCtx *BuildContext) ensureOutputDirExists(rootDir string) error {
	rootDir, _, err := types.DetectBuildContext(rootDir)
	if err != nil {
		return errors.Wrapf(err, "failed to detect build context")
	}
	outDirPath := path.Join(rootDir, types.BuildOutputDir)
	return os.MkdirAll(outDirPath, os.ModePerm)
}

// NewDeployContext initializes new deploy context
func NewDeployContext(ctx *BuildContext, envs []string) *DeployContext {
	return &DeployContext{
		BuildContext: ctx,
		Envs:         envs,
	}
}

// NewBuildContext creates new build context
func NewBuildContext(ctx *BuildContext, logger util.Logger) *BuildContext {
	res := BuildContext{CommonCtx: types.NewCommonContext(ctx.CommonCtx, logger)}
	copy(res.Modules, ctx.Modules)
	copy(res.Profiles, ctx.Profiles)
	res.InitGitClientIfNeeded()
	res.CancelOnSignal()
	return &res
}

// ToBuildCtx creates build context out of Micros context
func (m *MicrosCtx) ToBuildCtx() *BuildContext {
	return NewBuildContext(&BuildContext{CommonCtx: types.NewCommonContext(&types.CommonCtx{
		Parallel:         m.Parallel,
		Modules:          m.Modules,
		SoxEnabled:       m.SoxEnabled,
		SkipTestsEnabled: m.SkipTestsEnabled,
	}, m.Logger())}, m.Logger())
}
