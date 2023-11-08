package welder

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"runtime"
)

// ActualDeployDefinitionFor builds effective deploy definition for module
func (buildCtx *BuildContext) ActualDeployDefinitionFor(root *types.RootBuildDefinition, moduleName string, deployCtx *DeployContext) (types.DeployDefinition, types.ModuleDefinition, error) {
	module, err := root.RawModuleConfig(moduleName)
	if err != nil {
		return types.DeployDefinition{}, module, err
	}

	// return cached version
	buildHash, err := buildCtx.CalcHash()
	if err != nil {
		return types.DeployDefinition{}, module, errors.Wrapf(err, "failed to calc build context hash for module %s", moduleName)
	}
	deployHash, err := buildCtx.CalcHash()
	if err != nil {
		return types.DeployDefinition{}, module, errors.Wrapf(err, "failed to calc deploy context hash for module %s", moduleName)
	}
	cacheKey := fmt.Sprintf("%s:%s:%s", buildHash, deployHash, moduleName)
	if cachedDef, ok := root.GetCachedDeployDef(cacheKey); ok {
		return cachedDef, module, nil
	}

	deploy := module.Deploy.Init()
	tpl := Tpl{buildCtx: buildCtx, root: root, module: &module, deployCtx: deployCtx}
	allEnvironments := make(types.DeployEnvsDefinition)
	for k, v := range deploy.Environments {
		v.CommonRunDefinition.Init()
		allEnvironments[k] = v
	}
	buildCtx.MergeDeployEnvironments(root, &allEnvironments, true)

	// TODO: only 1 environment currently supported
	if deployCtx != nil && len(deployCtx.Envs) > 0 {
		env := deployCtx.Envs[0]
		types.MergeRunDefinitions(allEnvironments[env].CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	}

	// inherit everything from build section of a module
	types.MergeRunDefinitions(module.Build.CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	// inherit everything that hasn't been defined from build definition
	if err := tpl.calcActualBuildDefinitionFor(&deploy.BuildDefinition, true); err != nil {
		return *deploy, module, err
	}
	types.MergeSimpleRunDefinitions(*module.Deploy.CommonSimpleRunDefinition.Init(), &deploy.BuildDefinition.CommonSimpleRunDefinition, false)
	types.MergeRunDefinitions(*module.Build.CommonRunDefinition.Init(), &deploy.BuildDefinition.CommonRunDefinition, false)
	for _, profile := range buildCtx.ActiveProfiles(root, moduleName) {
		types.MergeRunDefinitions(root.Profiles[profile].Deploy.CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	}
	for _, profile := range buildCtx.ActiveProfiles(root, moduleName) {
		types.MergeRunDefinitions(root.Profiles[profile].Build.CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	}
	types.MergeRunDefinitions(root.Default.Deploy.CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	types.MergeRunDefinitions(root.Default.Build.CommonRunDefinition, &deploy.BuildDefinition.CommonRunDefinition, false)
	if len(deploy.Steps) > 0 { // set environments only if there are step definitions
		deploy.Environments = allEnvironments
	}
	if err := tpl.applyTemplatesWithMarshalling(deploy); err != nil {
		return *deploy, module, err
	}
	deploy.Init()

	root.CacheDeployDef(cacheKey, *deploy)

	return *deploy, module, err
}

// ActualSimpleStepsDefinitionFor builds effective simple Steps
func ActualSimpleStepsDefinitionFor(root *types.RootBuildDefinition, ctx *BuildContext,
	module *types.ModuleDefinition, step *types.SimpleStepDefinition) (types.SimpleStepDefinition, error) {
	tpl := Tpl{buildCtx: ctx, root: root, module: module}
	res, err := step.Clone()
	if err != nil {
		return res, err
	}
	if err := tpl.applyTemplatesWithMarshalling(&res); err != nil {
		return res, err
	}
	return res, err
}

// ActualBuildDefinitionFor builds effective build definition for a specified module
func (buildCtx *BuildContext) ActualBuildDefinitionFor(root *types.RootBuildDefinition, moduleName string) (types.BuildDefinition, types.ModuleDefinition, error) {
	module, err := root.RawModuleConfig(moduleName)
	if err != nil {
		return types.BuildDefinition{}, module, err
	}

	// return cached version
	ctxHash, err := buildCtx.CalcHash()
	if err != nil {
		return types.BuildDefinition{}, module, errors.Wrapf(err, "failed to calc build context hash for module %s", moduleName)
	}
	cacheKey := fmt.Sprintf("%s:%s", ctxHash, moduleName)
	if cachedDef, ok := root.GetCachedBuildDef(cacheKey); ok {
		return cachedDef, module, nil
	}

	build := module.Build.Init()
	tpl := Tpl{buildCtx: buildCtx, root: root, module: &module}
	err = tpl.calcActualBuildDefinitionFor(build, false)

	// cache calculated version
	root.CacheBuildDef(cacheKey, *build)

	return *build, module, err
}

// ActualTaskDefinitionForRawTask builds effective build definition for a provided raw task definition
func (buildCtx *BuildContext) ActualTaskDefinitionForRawTask(root *types.RootBuildDefinition, taskName string, task types.TaskDefinition,
	moduleName string, deployCtx *DeployContext) (types.TaskDefinition, error) {
	// return cached version
	buildHash, err := buildCtx.CalcHash()
	if err != nil {
		return types.TaskDefinition{}, errors.Wrapf(err, "failed to calc build context hash for module %s and task %s", moduleName, taskName)
	}
	deployHash, err := buildCtx.CalcHash()
	if err != nil {
		return types.TaskDefinition{}, errors.Wrapf(err, "failed to calc deploy context hash for module %s and task %s", moduleName, taskName)
	}
	cacheKey := fmt.Sprintf("%s:%s:%s:%s", buildHash, deployHash, taskName, moduleName)
	if cachedDef, ok := root.GetCachedTaskDef(cacheKey); ok {
		return cachedDef, nil
	}

	task.Init()
	var build *types.BuildDefinition
	var tpl *Tpl
	if moduleName != "" {
		if buildRunCtx, err := buildCtx.calcModuleBuildRunContext(root, moduleName, deployCtx); err != nil {
			return types.TaskDefinition{}, err
		} else {
			tpl = &Tpl{buildCtx: buildRunCtx.buildCtx, root: root, deployCtx: deployCtx, module: buildRunCtx.module}
			build = &buildRunCtx.buildDef
		}
	} else {
		build = &types.BuildDefinition{CommonRunDefinition: task.CommonRunDefinition}
		tpl = &Tpl{buildCtx: buildCtx, root: root, deployCtx: deployCtx}
		if err := tpl.calcActualBuildDefinitionFor(build, deployCtx != nil); err != nil {
			return types.TaskDefinition{}, err
		}
	}
	for argName, argVal := range buildCtx.BuildArgs {
		if argVal != "" {
			task.Args[argName] = types.StringValue(tpl.applyTemplate(string(argVal)))
		}
	}
	types.MergeRunDefinitions(task.CommonRunDefinition, &build.CommonRunDefinition, false)
	if err := tpl.applyTemplatesWithMarshalling(&build); err != nil {
		return types.TaskDefinition{}, err
	}
	types.OverrideMapValuesIfNotEmpty(task.Args, tpl.buildCtx.BuildArgs)
	if err := tpl.applyTemplatesWithMarshalling(&task); err != nil {
		return types.TaskDefinition{}, err
	}
	types.MergeRunDefinitions(build.CommonRunDefinition, &task.CommonRunDefinition, false)
	for scriptIndex, script := range task.Scripts {
		task.Scripts[scriptIndex] = tpl.applyTemplate(script)
	}
	task.Image = tpl.applyTemplate(task.Image)
	types.MergeMapIfEmpty(task.Args, tpl.buildCtx.BuildArgs)
	if err := tpl.applyTemplatesWithMarshalling(&task); err != nil {
		return types.TaskDefinition{}, err
	}
	// cache calculated version
	root.CacheTaskDef(cacheKey, task)
	return task, err
}

// ActualTaskDefinitionFor builds effective build definition for a specified module
func (buildCtx *BuildContext) ActualTaskDefinitionFor(root *types.RootBuildDefinition, taskName string, moduleName string, deployCtx *DeployContext) (types.TaskDefinition, error) {
	task, err := root.RawTaskConfig(taskName)
	if err != nil {
		return types.TaskDefinition{}, err
	}
	return buildCtx.ActualTaskDefinitionForRawTask(root, taskName, task, moduleName, deployCtx)
}

// ActualDockerImagesDefinitionFor reads build config for a specific module and builds effective docker definitions list
func (buildCtx *BuildContext) ActualDockerImagesDefinitionFor(root *types.RootBuildDefinition, moduleName string) ([]types.DockerImageDefinition, error) {
	module, err := root.RawModuleConfig(moduleName)
	if err != nil {
		return nil, err
	}

	// return cached version
	buildHash, err := buildCtx.CalcHash()
	if err != nil {
		return []types.DockerImageDefinition{}, errors.Wrapf(err, "failed to calc build context hash for module %s", moduleName)
	}
	cacheKey := fmt.Sprintf("%s:%s", buildHash, moduleName)
	if cachedDef, ok := root.GetCachedDockerDef(cacheKey); ok {
		return cachedDef, nil
	}

	defaultValues, err := root.Default.Clone()
	if err != nil {
		return nil, err
	}

	buildDef, _, err := buildCtx.ActualBuildDefinitionFor(root, moduleName)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to calculate dor images definitions for module %s", moduleName)
	}
	buildCtx.BuildArgs = buildDef.Args

	if len(module.DockerImages) == 0 {
		activeProfiles := buildCtx.ActiveProfiles(root, moduleName)
		for _, profile := range activeProfiles {
			if len(root.Profiles[profile].DockerImages) > 0 {
				module.DockerImages = root.Profiles[profile].DockerImages
				break
			}
		}
	}

	if len(module.DockerImages) == 0 {
		module.DockerImages = make([]types.DockerImageDefinition, 0)
		for _, di := range defaultValues.DockerImages {
			module.DockerImages = append(module.DockerImages, di)
		}
	}

	res := make([]types.DockerImageDefinition, len(module.DockerImages))
	tpl := Tpl{buildCtx: buildCtx, root: root, module: &module}
	for i, dockerImage := range module.DockerImages {
		for argIdx, arg := range dockerImage.Build.Args {
			dockerImage.Build.Args[argIdx].Value = tpl.applyTemplate(arg.Value)
			dockerImage.Build.Args[argIdx].File = tpl.applyTemplate(arg.File)
		}
		if err := tpl.applyTemplatesWithMarshalling(&dockerImage); err != nil {
			return nil, err
		}
		res[i] = dockerImage
	}
	if err := tpl.applyTemplatesWithMarshalling(&res); err != nil {
		return nil, err
	}

	// cache calculated version
	root.CacheDockerDef(cacheKey, res)

	return res, nil
}

// ActiveProfiles returns list of active profile names
func (buildCtx *BuildContext) ActiveProfiles(root *types.RootBuildDefinition, moduleName string) []string {
	activeProfiles := make([]string, 0)
	for _, profileName := range buildCtx.Profiles {
		if _, ok := root.Profiles[profileName]; ok {
			activeProfiles = append(activeProfiles, profileName)
		}
	}
	for profileName, profile := range root.Profiles {
		sox := profile.Activation.Sox && buildCtx.SoxEnabled
		skipTests := profile.Activation.SkipTests && buildCtx.SkipTestsEnabled
		bamboo := profile.Activation.Bamboo && buildCtx.IsRunningInBamboo()
		verbose := profile.Activation.Verbose && buildCtx.Verbose
		strict := profile.Activation.Strict && buildCtx.Strict
		parallel := profile.Activation.Parallel && buildCtx.Parallel
		pipelines := profile.Activation.Pipelines && buildCtx.IsRunningInBitbucketPipelines()
		linux := profile.Activation.Linux && (runtime.GOOS == "linux" || buildCtx.SimulateOS == "linux")
		darwin := profile.Activation.Darwin && (runtime.GOOS == "darwin" || buildCtx.SimulateOS == "darwin")
		windows := profile.Activation.Windows && (runtime.GOOS == "windows" || buildCtx.SimulateOS == "windows")
		if sox || skipTests || bamboo || verbose || strict || parallel || pipelines || linux || darwin || windows {
			activeProfiles = util.AddIfNotExist(activeProfiles, profileName)
		}
		if len(profile.Activation.If) > 0 && buildCtx.SubResolveContextDepth("profiles") < 5 {
			buildCtx.IncrementSubResolveContextDepth("profiles")
			var module *types.ModuleDefinition
			if len(moduleName) > 0 {
				if activeModule, err := root.RawModuleConfig(moduleName); err == nil {
					module = &activeModule
				}
			}
			tpl := Tpl{buildCtx: buildCtx, root: root, module: module}
			if res, err := tpl.evalToBool(profile.Activation.If); err == nil && res {
				activeProfiles = util.AddIfNotExist(activeProfiles, profileName)
			}
			buildCtx.DecrementSubResolveContextDepth("profiles")
		}
	}
	return activeProfiles
}

// CheckRunCondition checks whether this task needs to run
func CheckRunCondition(root *types.RootBuildDefinition, ctx BuildContext, moduleName string, spec types.RunSpec) (bool, error) {
	var module *types.ModuleDefinition
	if moduleName != "" {
		if moduleDef, err := root.RawModuleConfig(moduleName); err != nil {
			return false, err
		} else {
			module = &moduleDef
		}
	}
	tpl := Tpl{buildCtx: &ctx, root: root, module: module}

	return tpl.evalToBool(spec.RunIf)
}

func (buildCtx *BuildContext) MergeDeployEnvironments(root *types.RootBuildDefinition, envs *types.DeployEnvsDefinition, isDeployment bool) {
	activeProfiles := buildCtx.ActiveProfiles(root, "")
	for _, profile := range activeProfiles {
		var profileData types.DeployEnvsDefinition
		if isDeployment {
			profileData = root.Profiles[profile].Deploy.Environments
		}
		for env, envConfig := range profileData {
			envs.AddIfNotExist(env, envConfig)
			resEnvs := *envs
			resEnv := resEnvs[env]
			types.MergeRunDefinitions(profileData[env].CommonRunDefinition, resEnv.Init(), false)
			resEnvs[env] = resEnv
		}
	}
	var defaultData types.DeployEnvsDefinition
	if isDeployment {
		defaultData = root.Default.Deploy.Environments
	}
	for env, envConfig := range defaultData {
		envConfig.Init()
		envs.AddIfNotExist(env, envConfig)
	}
	for env := range defaultData {
		resEnvs := *envs
		resEnv := resEnvs[env]
		types.MergeRunDefinitions(defaultData[env].CommonRunDefinition, resEnv.Init(), false)
		resEnvs[env] = resEnv
	}
}

// IsProfileActive returns true if profile was activated
func (buildCtx *BuildContext) IsProfileActive(name string, root *types.RootBuildDefinition) bool {
	profiles := buildCtx.ActiveProfiles(root, "")
	for _, p := range profiles {
		if name == p {
			return true
		}
	}
	return false
}
