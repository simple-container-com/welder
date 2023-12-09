package welder

import (
	"fmt"
	"path"
	"regexp"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/util/yamledit"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type VersionCtx struct {
	buildCtx     *BuildContext
	root         types.RootBuildDefinition
	contextRoot  string
	yaml         yamledit.YamlEdit
	activeModule *types.ModuleDefinition
	deployCtx    *DeployContext // optional deploy context (only available when deploy is active)
}

const (
	semverRegex = `(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`
)

var fullVerRegex = regexp.MustCompile(fmt.Sprintf("(?P<pre>(^([0-9]+))*)(?P<version>%s)(?P<post>.*)", semverRegex))

type versionInfo struct {
	preString     string
	versionString string
	postString    string

	semver semver.Version
}

// NewVersionCtx
func NewVersionCtx(buildCtx *BuildContext, root *types.RootBuildDefinition, module *types.ModuleDefinition) (*VersionCtx, error) {
	res := VersionCtx{
		yaml: yamledit.YamlEdit{
			SkipNotExisting: false,
			WriteInPlace:    true,
		},
	}

	moduleDef, rootDef, err := types.ReadBuildModuleDefinition(buildCtx.RootDir())
	if err != nil {
		return nil, err
	}
	if root == nil {
		root = &rootDef
	}
	if module == nil {
		module = moduleDef
	}
	res.activeModule = module
	activeModules := buildCtx.ActiveModules(*root, module)
	if len(activeModules) == 1 {
		moduleDef, err := root.RawModuleConfig(activeModules[0])
		if err != nil {
			return &res, err
		}
		res.activeModule = &moduleDef
	} else if module != nil {
		res.activeModule = module
	}
	res.root = rootDef
	res.root = *root
	res.buildCtx = buildCtx
	return &res, nil
}

// unresolvedVersion returns unresolved version for current context
func (ctx *VersionCtx) unresolvedVersion() (string, error) {
	ctx.resolveVersionArgs()
	// return module's version if defined
	if ctx.activeModule != nil {
		res, err := ctx.activeModule.ActualVersion(ctx.root)
		if err != nil {
			return res, err
		}
		return res, nil
	}
	// otherwise return project's version if defined
	if ctx.root.Version != "" {
		return ctx.root.Version, nil
	}
	return "", fmt.Errorf("version is empty")
}

// Version returns version defined for current context
func (ctx *VersionCtx) Version() (string, error) {
	empty := ""
	tpl := &Tpl{buildCtx: ctx.buildCtx, root: &ctx.root, module: ctx.activeModule, deployCtx: ctx.deployCtx}
	unresolvedVersion, err := ctx.unresolvedVersion()
	if err != nil {
		return empty, err
	}
	tpl.version = &empty

	ctx.buildCtx.IncrementSubResolveContextDepth("version")
	if ctx.buildCtx.SubResolveContextDepth("version") > 4 {
		return unresolvedVersion, nil
	}
	defer ctx.buildCtx.DecrementSubResolveContextDepth("version")

	if ctx.deployCtx != nil {
		var deployDef types.DeployDefinition
		if ctx.activeModule != nil {
			deployDef, _, err = ctx.buildCtx.ActualDeployDefinitionFor(&ctx.root, ctx.activeModule.Name, ctx.deployCtx)
		} else {
			deployDef = ctx.root.Default.Deploy
		}
		err := tpl.calcActualBuildDefinitionFor(&deployDef.BuildDefinition, true)
		if err != nil {
			return "", errors.Wrapf(err, "failed to calculate actual deploy definition")
		}
		tpl.buildCtx.BuildArgs = deployDef.Args
		tpl.deployCtx.BuildContext = tpl.buildCtx
	} else {
		var buildDef types.BuildDefinition
		if ctx.activeModule != nil {
			buildDef, _, err = ctx.buildCtx.ActualBuildDefinitionFor(&ctx.root, ctx.activeModule.Name)
		} else {
			buildDef = ctx.root.Default.Build
		}
		err := tpl.calcActualBuildDefinitionFor(&buildDef, false)
		if err != nil {
			return "", errors.Wrapf(err, "failed to calculate actual build definition")
		}
		tpl.buildCtx.BuildArgs = buildDef.Args
	}
	tpl.extraVars = util.Data{"project:root.version": tpl.applyTemplate(ctx.root.Version)}
	res := tpl.applyTemplate(unresolvedVersion)
	return res, nil
}

func (ctx *VersionCtx) resolveVersionArgs() {
	if ctx.buildCtx.BuildArgs == nil {
		ctx.buildCtx.BuildArgs = make(types.BuildArgs)
	}
	tmpCtx := NewBuildContext(ctx.buildCtx, ctx.buildCtx.Logger())
	resolvedVersion := ""
	tmpTpl := Tpl{buildCtx: tmpCtx, root: &ctx.root, module: ctx.activeModule, version: &resolvedVersion}
	// hack to support default/profile args in version
	activeModuleName := ""
	if ctx.activeModule != nil {
		moduleValues := ctx.activeModule.Build
		activeModuleName = ctx.activeModule.Name
		types.MergeMapIfEmpty(moduleValues.Args, ctx.buildCtx.BuildArgs)
	}
	activeProfiles := tmpTpl.buildCtx.ActiveProfiles(&ctx.root, activeModuleName)
	for _, profile := range activeProfiles {
		profileValues := ctx.root.Profiles[profile].Build
		types.MergeMapIfEmpty(profileValues.Args, ctx.buildCtx.BuildArgs)
	}
	defaultValues := ctx.root.Default.Build
	types.MergeMapIfEmpty(defaultValues.Args, ctx.buildCtx.BuildArgs)
}

// parseSemVer parses version from build config as a SemVer
func (ctx *VersionCtx) parseSemVer() (versionInfo, error) {
	result := versionInfo{}
	version, err := ctx.unresolvedVersion()
	if err != nil {
		return result, errors.Wrapf(err, "failed to get version")
	}
	match := fullVerRegex.FindStringSubmatch(version)
	if match == nil {
		return result, fmt.Errorf("invalid version format: %s, must match regexp: %s", version, fullVerRegex.String())
	}
	for i, name := range fullVerRegex.SubexpNames() {
		switch name {
		case "version":
			result.versionString = match[i]
		case "pre":
			result.preString = match[i]
		case "post":
			result.postString = match[i]
		}
	}
	if v, err := semver.NewVersion(result.versionString); err != nil {
		return result, errors.Wrapf(err, "failed to parse version string %q", result.versionString)
	} else {
		result.semver = *v
	}
	return result, nil
}

// toVersionString serializes actual semver representation back to string with post/pre text allowed
func (v *versionInfo) toVersionString() string {
	return v.preString + v.semver.String() + v.postString
}

// BumpMajor Bump major version
func (ctx *VersionCtx) BumpMajor() error {
	v, err := ctx.parseSemVer()
	if err != nil {
		return err
	}
	v.semver = v.semver.IncMajor()
	return ctx.SetVersionInConfig(v.toVersionString())
}

// BumpMinor Bump minor version
func (ctx *VersionCtx) BumpMinor() error {
	v, err := ctx.parseSemVer()
	if err != nil {
		return err
	}
	v.semver = v.semver.IncMinor()
	return ctx.SetVersionInConfig(v.toVersionString())
}

// BumpPatch Bump patch version
func (ctx *VersionCtx) BumpPatch() error {
	v, err := ctx.parseSemVer()
	if err != nil {
		return err
	}
	v.semver = v.semver.IncPatch()
	return ctx.SetVersionInConfig(v.toVersionString())
}

func (ctx *VersionCtx) SetVersionInConfig(version string) error {
	moduleIdx := -1
	if ctx.activeModule != nil {
		for idx, module := range ctx.root.ModuleNames() {
			if module == ctx.activeModule.Name {
				moduleIdx = idx
			}
		}
	}
	buildYamlFile := path.Join(ctx.root.RootDirPath(), types.BuildConfigFileName)
	path := "version"
	if moduleIdx >= 0 && (len(ctx.root.Modules) > 1 || (ctx.activeModule.Version != "" && ctx.root.Version == "")) {
		path = fmt.Sprintf("modules[%d].version", moduleIdx)
	}
	return ctx.yaml.ModifyProperty(buildYamlFile, path, version)
}
