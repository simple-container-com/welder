package welder

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/template"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

type Tpl struct {
	buildCtx  *BuildContext
	deployCtx *DeployContext
	root      *types.RootBuildDefinition
	module    *types.ModuleDefinition
	extraVars util.Data
	version   *string
}

func (tpl *Tpl) copyNonStrict() *Tpl {
	copyBuildCtx := NewBuildContext(tpl.buildCtx, tpl.buildCtx.Logger())
	copyBuildCtx.Strict = false
	copyTpl := Tpl{
		buildCtx:  copyBuildCtx,
		root:      tpl.root,
		module:    tpl.module,
		extraVars: tpl.extraVars,
		version:   tpl.version,
	}
	if tpl.deployCtx != nil {
		copyDepCtx := NewDeployContext(tpl.buildCtx, tpl.deployCtx.Envs)
		copyDepCtx.Strict = false
		copyTpl.deployCtx = copyDepCtx
	}
	return &copyTpl
}

func (tpl *Tpl) applyTemplate(value string) string {
	processedValue := tpl.initTemplate().Exec(value)
	tpl.buildCtx.Logger().Debugf("Processing placeholders in %q, result: %q", value, processedValue)
	return processedValue
}

func (tpl *Tpl) evalToBool(value string) (bool, error) {
	processedValue, err := tpl.initTemplate().EvalToBool(value)
	tpl.buildCtx.Logger().Debugf("Processing placeholders in %q, result: %t", value, processedValue)
	return processedValue, err
}

func (tpl *Tpl) initTemplate() *template.Template {
	data := util.Data{}
	data = tpl.addContainerTemplateVars(data)
	data = tpl.addHostTemplateVars(data)
	data.AddAllIfNotExist(tpl.extraVars)
	data = tpl.addDockerTemplateVars(data)
	return template.NewTemplate().
		WithGit(tpl.buildCtx.GitClient()).
		WithData(data).
		WithStrict(tpl.buildCtx.Strict).
		WithExtensions(map[string]template.Extension{
			"profile": tpl.extProfile,
			"mode":    tpl.extMode,
			"arg":     tpl.extArg,
			"project": tpl.extProject,
			"os":      tpl.extOS,
			"task":    tpl.extTask,
		})
}

// extTask enables placeholders like ${task:<name>.output} and ${task:<name>.success}
func (tpl *Tpl) extTask(noSubstitution, path string, defaultValue *string) (string, error) {
	var err error
	var moduleName string

	if tpl.module != nil {
		moduleName = tpl.module.Name
	}

	pathParts := strings.SplitN(path, ".", 2)
	taskName := pathParts[0]

	if tpl.buildCtx.IsExecutingTask(taskName) {
		return noSubstitution, errors.Errorf("failed to call task %s from itself, recursion is not allowed", taskName)
	}
	tpl.buildCtx.ExecutingTask(taskName)
	defer tpl.buildCtx.ExecutedTask(taskName)

	ctx := NewBuildContext(tpl.buildCtx, &util.NoopLogger{})
	action, err := ctx.ActualTaskDefinitionFor(tpl.root, taskName, moduleName, tpl.deployCtx)
	ctx.DecrementSubResolveContextDepth("task")
	if err != nil {
		return noSubstitution, errors.Wrapf(err, "failed to calcualate task definition for task %s", taskName)
	}

	convRun := action.ToRunSpec(taskName)

	taskErr := ctx.RunScripts(fmt.Sprintf("${task:%s}", taskName), tpl.root.ProjectNameOrDefault(), tpl.root, moduleName, convRun)

	if len(pathParts) == 1 {
		return strings.TrimSpace(ctx.LastExecOutput()), taskErr
	}
	taskErrMsg := ""
	if taskErr != nil {
		taskErrMsg = taskErr.Error()
	}
	res, err := util.GetValue(path, map[string]interface{}{
		taskName: map[string]interface{}{
			"trim":    strings.TrimSpace(ctx.LastExecOutput()),
			"raw":     ctx.LastExecOutput(),
			"error":   fmt.Sprintf("%s", taskErrMsg),
			"success": fmt.Sprintf("%t", taskErr == nil),
			"failed":  fmt.Sprintf("%t", taskErr != nil),
		},
	})
	if err != nil {
		return noSubstitution, err
	}
	return res.(string), nil
}

// extOS enables placeholders like ${os:type.linux} and ${os:name}
func (tpl *Tpl) extOS(noSubstitution, path string, defaultValue *string) (string, error) {
	res, err := util.GetValue(path, map[string]interface{}{
		"type": map[string]string{
			"darwin":  fmt.Sprintf("%t", runtime.GOOS == "darwin" || tpl.buildCtx.SimulateOS == "darwin"),
			"windows": fmt.Sprintf("%t", runtime.GOOS == "windows" || tpl.buildCtx.SimulateOS == "windows"),
			"linux":   fmt.Sprintf("%t", runtime.GOOS == "linux" || tpl.buildCtx.SimulateOS == "linux"),
		},
		"name": runtime.GOOS,
		"arch": runtime.GOARCH,
	})
	if err != nil {
		return noSubstitution, err
	}
	if res != nil {
		return res.(string), nil
	}
	return "", nil
}

// extProject enables placeholders like ${project:something}
func (tpl *Tpl) extProject(noSubstitution, path string, defaultValue *string) (string, error) {
	projectData := util.Data{
		"name": tpl.root.ProjectName,
		"root": tpl.root.ProjectRoot,
	}
	if tpl.version == nil {
		if verCtx, err := NewVersionCtx(tpl.buildCtx, tpl.root, tpl.module); err != nil {
			tpl.version = &tpl.root.Version
			if version, err := tpl.module.ActualVersion(*tpl.root); err == nil {
				tpl.version = &version
			}
		} else {
			verCtx.deployCtx = tpl.deployCtx
			if version, err := verCtx.Version(); err == nil {
				tpl.version = &version
			}
		}
	}
	if tpl.version != nil {
		projectData["version"] = *tpl.version
	}
	if tpl.module != nil {
		projectData["module"] = map[string]string{
			"path": tpl.module.Path,
			"name": tpl.module.Name,
		}
	}
	// TODO: only 1 environment currently supported
	if tpl.deployCtx != nil && len(tpl.deployCtx.Envs) > 0 {
		env := tpl.deployCtx.Envs[0]
		projectData["env"] = env
	}
	res, err := util.GetValue(path, projectData)
	if err != nil {
		if defaultValue != nil {
			return *defaultValue, nil
		}
		return noSubstitution, err
	}
	return res.(string), nil
}

// extArg enables placeholders like ${arg:some-argument}
func (tpl *Tpl) extArg(noSubstitution, path string, defaultValue *string) (string, error) {
	for k, v := range tpl.buildCtx.BuildArgs {
		if path == k {
			return string(v), nil
		}
	}
	if defaultValue != nil {
		return *defaultValue, nil
	}
	return noSubstitution, nil
}

// extMode enables placeholders like ${mode:sox}, {mode:bamboo} or ${mode:skip-tests}
func (tpl *Tpl) extMode(noSubstitution, path string, defaultValue *string) (string, error) {
	switch path {
	case "bamboo":
		return strconv.FormatBool(tpl.buildCtx.IsRunningInBamboo()), nil
	case "pipelines":
		return strconv.FormatBool(tpl.buildCtx.IsRunningInBitbucketPipelines()), nil
	case "ci":
		return strconv.FormatBool(tpl.buildCtx.IsRunningInCI()), nil
	case "sox":
		return strconv.FormatBool(tpl.buildCtx.SoxEnabled), nil
	case "skip-tests":
		return strconv.FormatBool(tpl.buildCtx.SkipTestsEnabled), nil
	case "on-host":
		return strconv.FormatBool(tpl.buildCtx.ForceOnHost), nil
	case "verbose":
		return strconv.FormatBool(tpl.buildCtx.Verbose), nil
	case "no-cache":
		return strconv.FormatBool(tpl.buildCtx.NoCache), nil
	case "sync-mode":
		return string(tpl.buildCtx.SyncMode), nil
	default:
		if defaultValue != nil {
			return *defaultValue, nil
		}
		return noSubstitution, nil
	}
}

// extProfile enables placeholders like ${profile:bamboo.active}
func (tpl *Tpl) extProfile(noSubstitution, path string, defaultValue *string) (string, error) {
	parts := strings.Split(path, ".")
	if len(parts) != 2 {
		return "", errors.Errorf("invalid path: %q", path)
	}
	profile := parts[0]
	action := parts[1]
	switch action {
	case "active":
		return strconv.FormatBool(tpl.buildCtx.IsProfileActive(profile, tpl.root)), nil
	}
	if defaultValue != nil {
		return *defaultValue, nil
	}
	return noSubstitution, nil
}

func (tpl *Tpl) addContainerTemplateVars(data util.Data) util.Data {
	home := "/home/" + tpl.buildCtx.Username

	if tpl.buildCtx.Username == "root" || tpl.buildCtx.Username == "" {
		home = "/root"
	}
	add := util.Data{
		"container:home": home,
	}
	for k, v := range add {
		data[k] = v
	}
	return data
}

func (tpl *Tpl) addHostTemplateVars(data util.Data) util.Data {
	wd, err := os.Getwd()
	if err != nil {
		panic(errors.Wrapf(err, "failed to detect working dir"))
	}
	add := util.Data{
		"host:wd":          wd,
		"host:projectRoot": tpl.root.ProjectRoot,
	}
	for k, v := range add {
		data[k] = v
	}
	return data
}

func (tpl *Tpl) addDockerTemplateVars(data util.Data) util.Data {
	if tpl.buildCtx.CurrentDockerImage() != nil {
		dockerImage := tpl.buildCtx.CurrentDockerImage()
		data["docker:image"] = dockerImage.Name
		for i, digest := range dockerImage.Digests {
			data[fmt.Sprintf("docker:tags[%d].tag", i)] = digest.Tag
			data[fmt.Sprintf("docker:tags[%d].digest", i)] = digest.Digest
			data[fmt.Sprintf("docker:tags[%d].image", i)] = digest.Image
		}
	}
	return data
}
func (tpl *Tpl) applyTemplateOnValuesMap(target map[string]types.StringValue) {
	for k, v := range target {
		target[k] = types.StringValue(tpl.applyTemplate(string(v)))
	}
}

func (tpl *Tpl) applyTemplateOnValues(target []string) {
	for k, v := range target {
		target[k] = tpl.applyTemplate(v)
	}
}

func (tpl *Tpl) calcActualBuildDefinitionFor(res *types.BuildDefinition, isDeployment bool) error {
	// args have priority
	types.MergeMapIfEmpty(res.Args, tpl.buildCtx.BuildArgs)
	if tpl.buildCtx.BuildArgs != nil {
		res.Args = tpl.buildCtx.BuildArgs
	}
	// apply values from profiles if any
	activeProfiles := tpl.buildCtx.ActiveProfiles(tpl.root, tpl.ActiveModuleName())
	for _, profile := range activeProfiles {
		if isDeployment {
			types.MergeRunDefinitions(tpl.root.Profiles[profile].Deploy.BuildDefinition.CommonRunDefinition, &res.CommonRunDefinition, false)
			if err := types.MergeSteps(tpl.root.Profiles[profile].Deploy.BuildDefinition, res); err != nil {
				return err
			}
		}
		types.MergeRunDefinitions(tpl.root.Profiles[profile].Build.CommonRunDefinition, &res.CommonRunDefinition, false)
		if err := types.MergeSteps(tpl.root.Profiles[profile].Build, res); err != nil {
			return err
		}
	}
	// inherit the rest from defaults
	if isDeployment {
		types.MergeRunDefinitions(*tpl.root.Default.Deploy.CommonRunDefinition.Init(), &res.CommonRunDefinition, false)
		if err := types.MergeSteps(tpl.root.Default.Deploy.BuildDefinition, res); err != nil {
			return err
		}
	}
	types.MergeRunDefinitions(*tpl.root.Default.Build.CommonRunDefinition.Init(), &res.CommonRunDefinition, false)
	if err := types.MergeSteps(tpl.root.Default.Build, res); err != nil {
		return err
	}

	// apply templates to all values
	// hack: apply a few times so that cross-ref variables resolved (disabling strict mode)
	// it'd be better if we check whether some placeholders are still unresolved instead
	for i := 0; i < 3; i++ {
		tpl.copyNonStrict().applyTemplateOnValuesMap(res.Args)
		tpl.copyNonStrict().applyTemplateOnValuesMap(res.Env)
		tpl.buildCtx.BuildArgs = res.Args
	}
	return tpl.applyTemplatesWithMarshalling(res)
}

func (tpl *Tpl) applyTemplatesWithMarshalling(out interface{}) error {
	reflectedVal := reflect.ValueOf(out).Elem()
	appliedResult := tpl.applyTemplates(out)
	val := reflect.ValueOf(appliedResult).Elem()
	reflectedVal.Set(val)
	return nil
}

func (tpl *Tpl) applyTemplates(obj interface{}) interface{} {
	// Wrap the original in a reflect.Value
	original := reflect.ValueOf(obj)

	res := reflect.New(original.Type()).Elem()
	tpl.applyTemplatesRecursive(res, original)

	// Remove the reflection wrapper
	return res.Interface()
}

func (tpl *Tpl) applyTemplatesRecursive(copy, original reflect.Value) {
	switch original.Kind() {
	// The first cases handle nested structures and translate them recursively

	// If it is a pointer we need to unwrap and call once again
	case reflect.Ptr:
		// To get the actual value of the original we have to call Elem()
		// At the same time this unwraps the pointer so we don't end up in
		// an infinite recursion
		originalValue := original.Elem()
		// Check if the pointer is nil
		if !originalValue.IsValid() {
			return
		}
		// Allocate a new object and set the pointer to it
		copy.Set(reflect.New(originalValue.Type()))
		// Unwrap the newly created pointer
		tpl.applyTemplatesRecursive(copy.Elem(), originalValue)

	// If it is an interface (which is very similar to a pointer), do basically the
	// same as for the pointer. Though a pointer is not the same as an interface so
	// note that we have to call Elem() after creating a new object because otherwise
	// we would end up with an actual pointer
	case reflect.Interface:
		// Get rid of the wrapping interface
		originalValue := original.Elem()
		// Create a new object. Now new gives us a pointer, but we want the value it
		// points to, so we have to call Elem() to unwrap it
		copyValue := reflect.New(originalValue.Type()).Elem()
		tpl.applyTemplatesRecursive(copyValue, originalValue)
		copy.Set(copyValue)

	// If it is a struct we translate each field
	case reflect.Struct:
		for i := 0; i < original.NumField(); i += 1 {
			tpl.applyTemplatesRecursive(copy.Field(i), original.Field(i))
		}

	// If it is a slice we create a new slice and translate each element
	case reflect.Slice:
		copy.Set(reflect.MakeSlice(original.Type(), original.Len(), original.Cap()))
		for i := 0; i < original.Len(); i += 1 {
			tpl.applyTemplatesRecursive(copy.Index(i), original.Index(i))
		}

	// If it is a map we create a new map and translate each value
	case reflect.Map:
		copy.Set(reflect.MakeMap(original.Type()))
		for _, key := range original.MapKeys() {
			originalValue := original.MapIndex(key)
			// New gives us a pointer, but again we want the value
			copyValue := reflect.New(originalValue.Type()).Elem()
			tpl.applyTemplatesRecursive(copyValue, originalValue)
			copy.SetMapIndex(key, copyValue)
		}

	// Otherwise we cannot traverse anywhere so this finishes the recursion

	// If it is a string translate it (yay finally we're doing what we came for)
	case reflect.String:
		var processed string
		originalVal := original.Interface()
		if _, ok := originalVal.(string); ok {
			processed = tpl.copyNonStrict().applyTemplate(originalVal.(string))
		} else if _, ok := originalVal.(types.StringValue); ok {
			processed = tpl.copyNonStrict().applyTemplate(string(originalVal.(types.StringValue)))
		} else {
			processed = tpl.copyNonStrict().applyTemplate(string(originalVal.(types.RunOnType)))
		}
		copy.SetString(processed)

	// And everything else will simply be taken from the original
	default:
		copy.Set(original)
	}
}

func (tpl *Tpl) ActiveModuleName() string {
	if tpl.module != nil {
		return tpl.module.Name
	}
	return ""
}
