package welder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"

	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/exec"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/runner"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

type KanikoOpts struct {
	ExecutorPath string
	CachePath    string
	ExtraArgs    string
	DockerImages []string
}

type dockerBuildParams struct {
	dockerImage DockerImageDefinition
	subject     string
	allowReuse  bool
	kanikoOpts  *KanikoOpts
	allowLabels bool
}

// KanikoBuild builds docker images using Kaniko executor
func (buildCtx *BuildContext) KanikoBuild(opts KanikoOpts) error {
	return buildCtx.forEachModule("building Docker images", func(root *RootBuildDefinition, modCtx *BuildContext, module string) error {
		buildCtx.Logger().Logf(" - Building Docker images for module '%s'...", module)
		dockerDefs, err := modCtx.ActualDockerImagesDefinitionFor(root, module)
		if err != nil {
			return errors.Wrapf(err, "failed to calc effective Docker images definition for module %s", module)
		}
		for _, dockerDef := range dockerDefs {
			if len(opts.DockerImages) > 0 && !util.SliceContains(opts.DockerImages, dockerDef.Name) {
				continue
			}
			subCtx := NewBuildContext(modCtx, modCtx.Logger().SubLogger(dockerDef.Name))
			if _, err := subCtx.buildDockerImage(root, module, dockerBuildParams{
				dockerImage: dockerDef,
				subject:     dockerDef.Name,
				allowReuse:  false,
				kanikoOpts:  &opts,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// BuildDocker builds docker images defined by the build context
func (buildCtx *BuildContext) BuildDocker(dockerImages []string) error {
	return buildCtx.forEachModule("building Docker images", func(root *RootBuildDefinition, modCtx *BuildContext, module string) error {
		buildCtx.Logger().Logf(" - Building Docker images for module '%s'...", module)
		dockerDefs, err := modCtx.ActualDockerImagesDefinitionFor(root, module)
		if err != nil {
			return errors.Wrapf(err, "failed to calc effective Docker images definition for module %s", module)
		}
		for _, dockerDef := range dockerDefs {
			if len(dockerImages) > 0 && !util.SliceContains(dockerImages, dockerDef.Name) {
				continue
			}
			subCtx := NewBuildContext(modCtx, modCtx.Logger().SubLogger(dockerDef.Name))
			if _, err := subCtx.buildDockerImage(root, module, dockerBuildParams{
				dockerImage: dockerDef,
				subject:     dockerDef.Name,
				allowReuse:  false,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// PushDocker pushes built images to Docker registries
func (buildCtx *BuildContext) PushDocker(dockerImages []string) error {
	if err := buildCtx.ensureOutputDirExists(buildCtx.RootDir()); err != nil {
		return errors.Wrapf(err, "failed to create output dir")
	}
	outDockerDef := OutDockerDefinition{SchemaVersion: OutDockerSchemaVersion}
	err := buildCtx.forEachModule("pushing Docker images", func(root *RootBuildDefinition, modCtx *BuildContext, module string) error {
		outDockerModuleDef := OutDockerModuleDefinition{Name: module}
		buildCtx.Logger().Logf(" - Pushing Docker images for module '%s'...", module)
		dockerDefs, err := modCtx.ActualDockerImagesDefinitionFor(root, module)
		if err != nil {
			return err
		}
		for _, d := range dockerDefs {
			if len(dockerImages) > 0 && !util.SliceContains(dockerImages, d.Name) {
				continue
			}
			dockerDef := d
			subCtx := NewBuildContext(modCtx, modCtx.Logger().SubLogger(dockerDef.Name))
			subCtx.Logger().Logf(" - Pushing Docker image '%s'...", dockerDef.Name)

			pushedDockerImage := OutDockerImageDefinition{
				Name:    dockerDef.Name,
				Digests: make([]OutDockerDigestDefinition, 0),
			}

			for _, tag := range dockerDef.Tags {
				dockerfile, err := docker.NewDockerfile(buildCtx.GoContext(), root.PathTo(buildCtx.RootDir(), dockerDef.DockerFile), tag)
				if err != nil {
					return err
				}
				dockerfile.Context = buildCtx.GoContext()
				dockerfile.ContextPath = dockerDef.Build.ContextPath
				reader, err := dockerfile.Push()
				if err != nil {
					return err
				}
				if err := reader.Listen(false, docker.MessageToLogFunc(subCtx.Logger(), module)); err != nil {
					return err
				}
				for repoTag, digest := range dockerfile.TagDigests {
					image, err := docker.ImageFromReference(repoTag)
					if err != nil {
						return errors.Wrapf(err, "failed to determine image name from tag: %s", repoTag)
					}
					pushedDockerImage.Digests = append(pushedDockerImage.Digests, OutDockerDigestDefinition{
						Tag:    digest.Tag,
						Digest: digest.Digest,
						Image:  image,
					})
				}
			}
			if err := modCtx.runAfterPushScripts(root, module, dockerDef, pushedDockerImage); err != nil {
				return errors.Wrapf(err, "failed to run after push scripts")
			}
			outDockerModuleDef.DockerImages = append(outDockerModuleDef.DockerImages, pushedDockerImage)
		}
		outDockerDef.Modules = append(outDockerDef.Modules, outDockerModuleDef)
		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to Docker push")
	}
	buildCtx.Logger().Logf(" - Writing output file...")
	err = outDockerDef.WriteToOutputDir(buildCtx.RootDir())
	if err != nil {
		return errors.Wrapf(err, "failed to write output file")
	}
	return nil
}

func (buildCtx *BuildContext) buildDockerImage(root *RootBuildDefinition, moduleName string, buildParams dockerBuildParams) ([]string, error) {
	dockerFilePath := buildParams.dockerImage.DockerFile
	if dockerFilePath != "" && !path.IsAbs(dockerFilePath) {
		dockerFilePath = path.Join(buildCtx.RootDir(), dockerFilePath)
	}
	var tags []string
	tags = buildParams.dockerImage.Tags
	if len(tags) == 0 {
		tags = append(tags, buildParams.dockerImage.Name)
	}

	if buildParams.dockerImage.InlineDockerfile != "" {
		buildCtx.Logger().Logf(" - Building Docker image '%s' from inline dockerfile...", buildParams.dockerImage.Name)
		buildCtx.Logger().Debugf("InlineDockerFile: %s", buildParams.dockerImage.InlineDockerfile)
		tmpDir, err := ioutil.TempDir(os.TempDir(), "inline")
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to create temp directory")
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		tmpFile, err := os.CreateTemp(tmpDir, "Dockerfile")
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to create temp file in %s", tmpDir)
		}
		dockerFilePath = tmpFile.Name()
		if err := os.WriteFile(dockerFilePath, []byte(buildParams.dockerImage.InlineDockerfile), os.ModePerm); err != nil {
			return tags, errors.Wrapf(err, "failed to write Dockerfile to path %s", dockerFilePath)
		}
	}
	buildCtx.Logger().Logf(" - Building Docker image '%s' from file '%s'...", buildParams.dockerImage.Name, dockerFilePath)

	if buildParams.kanikoOpts != nil {
		if err := buildCtx.buildDockerImageWithKaniko(root, moduleName, dockerFilePath, tags, buildParams); err != nil {
			return tags, errors.Wrapf(err, "failed to build Docker image using Kaniko")
		}
	} else if err := buildCtx.buildDockerImageWithDocker(root, moduleName, dockerFilePath, tags, buildParams); err != nil {
		return tags, errors.Wrapf(err, "failed to build Docker image")
	}

	return tags, nil
}

func (buildCtx *BuildContext) runAfterPushScripts(root *RootBuildDefinition, module string, dockerDef DockerImageDefinition, pushedDockerImage OutDockerImageDefinition) error {
	pushSubCtx := NewBuildContext(buildCtx, buildCtx.Logger().SubLogger("after-push"))
	pushSubCtx.SetCurrentDockerImage(&pushedDockerImage)
	runId := fmt.Sprintf("after-push-%s", dockerDef.Name)
	tasks := dockerDef.RunAfterPush.Tasks
	if len(dockerDef.RunAfterPush.Scripts) > 0 {
		buildCtx.Logger().Logf(" - Running after Docker push step...")
		if err := pushSubCtx.RunScriptsOfSimpleStep(runId, dockerDef.RunAfterPush, root, module); err != nil {
			return errors.Wrapf(err, "failed to run after push script for %q", dockerDef.Name)
		}
	} else if len(tasks) > 0 {
		return pushSubCtx.runAfterTasks(runId, root, module, tasks)
	}
	return nil
}

func (buildCtx *BuildContext) runAfterTasks(runId string, root *RootBuildDefinition, module string, tasks []string) error {
	for _, task := range tasks {
		action, err := buildCtx.ActualTaskDefinitionFor(root, task, module, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to calcualate task definition for task %s of module %s", task, module)
		}
		runId = fmt.Sprintf("%s-%s", runId, task)
		convRun := action.ToRunSpec(task)
		buildCtx.ExecutingTask(task)
		if err := buildCtx.RunScripts(fmt.Sprintf("%s step %q of module %s", action, task, module), runId, root, module, convRun); err != nil {
			buildCtx.ExecutedTask(task)
			return err
		}
		buildCtx.ExecutedTask(task)
	}
	return nil
}

func (buildCtx *BuildContext) runAfterBuildScripts(root *RootBuildDefinition, moduleName string, buildParams dockerBuildParams, tags []string) error {
	afterDockerBuildCtx := NewBuildContext(buildCtx, buildCtx.Logger().SubLogger("after-build"))
	digests := make([]OutDockerDigestDefinition, 0)
	for _, tag := range tags {
		image, tag, err := docker.ImageAndTagFromFullReference(tag)
		if err != nil {
			return err
		}
		digests = append(digests, OutDockerDigestDefinition{Tag: tag, Image: image})
	}
	afterDockerBuildCtx.SetCurrentDockerImage(&OutDockerImageDefinition{Name: buildParams.dockerImage.Name, Digests: digests})
	runId := fmt.Sprintf("after-build-%s", buildParams.dockerImage.Name)
	runAfterBuild := buildParams.dockerImage.RunAfterBuild
	tasks := runAfterBuild.Tasks
	if len(runAfterBuild.Scripts) > 0 {
		buildCtx.Logger().Logf(" - Running after Docker build step...")
		if err := afterDockerBuildCtx.RunScriptsOfSimpleStep(
			runId,
			runAfterBuild, root, moduleName); err != nil {
			return errors.Wrapf(err, "failed to invoke after build script ")
		}
	} else if len(tasks) > 0 {
		return afterDockerBuildCtx.runAfterTasks(runId, root, moduleName, tasks)
	}
	return nil
}

func (buildCtx *BuildContext) buildDockerImageWithKaniko(root *RootBuildDefinition, module string, dockerFilePath string, tags []string, buildParams dockerBuildParams) error {
	if err := buildCtx.ensureOutputDirExists(buildCtx.RootDir()); err != nil {
		return errors.Wrapf(err, "failed to create output dir")
	}

	kanikoExec := exec.NewExec(buildCtx.GoContext(), buildCtx.Logger())
	args := []string{buildParams.kanikoOpts.ExecutorPath}

	if !buildCtx.NoCache {
		args = append(args, "--cache")
	}
	args = append(args, "--dockerfile", dockerFilePath)
	contextPath := buildParams.dockerImage.Build.ContextPath
	if contextPath == "" {
		contextPath = filepath.Dir(dockerFilePath)
	}

	args = append(args, "--context", contextPath)

	for _, tag := range tags {
		args = append(args, "--destination", tag)
	}

	if argsMap, err := buildParams.dockerImage.Build.ArgsToMap(); err != nil {
		return errors.Wrapf(err, "failed to convert build args")
	} else {
		for key, value := range argsMap {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, *value))
		}
	}

	digestFile := path.Join(buildCtx.RootDir(), BuildOutputDir, fmt.Sprintf("%s-%s-digest", module, buildParams.dockerImage.Name))
	args = append(args, "--digest-file", digestFile)

	extraArgs, err := shellquote.Split(buildParams.kanikoOpts.ExtraArgs)
	if err != nil {
		return errors.Wrapf(err, "failed to split extra args %q", buildParams.kanikoOpts.ExtraArgs)
	}
	args = append(args, extraArgs...)

	if _, err := kanikoExec.ExecCommandAndLog("kaniko", shellquote.Join(args...), exec.Opts{}); err != nil {
		return errors.Wrapf(err, "failed to invoke kaniko executor")
	}

	digest := ""
	if bytes, err := ioutil.ReadFile(digestFile); err != nil {
		return errors.Wrapf(err, "failed to read digest file %q", digestFile)
	} else {
		digest = string(bytes)
	}

	if err := buildCtx.runAfterBuildScripts(root, module, buildParams, tags); err != nil {
		return errors.Wrapf(err, "failed to invoke scripts after push")
	}

	pushedDockerImage := OutDockerImageDefinition{
		Name:    buildParams.dockerImage.Name,
		Digests: make([]OutDockerDigestDefinition, 0),
	}

	for _, ref := range tags {
		image, tag, err := docker.ImageAndTagFromFullReference(ref)
		if err != nil {
			return err
		}
		pushedDockerImage.Digests = append(pushedDockerImage.Digests, OutDockerDigestDefinition{
			Tag:    tag,
			Image:  image,
			Digest: digest,
		})
	}

	if err := buildCtx.runAfterPushScripts(root, module, buildParams.dockerImage, pushedDockerImage); err != nil {
		return errors.Wrapf(err, "failed to invoke scripts after push")
	}

	// TODO: write output push data

	return nil
}

func (buildCtx *BuildContext) buildDockerImageWithDocker(root *RootBuildDefinition, module string, dockerFilePath string, tags []string, buildParams dockerBuildParams) error {
	dockerFile, err := docker.NewDockerfile(buildCtx.GoContext(), dockerFilePath, tags...)
	if err != nil {
		return errors.Wrapf(err, "failed to init Dockerfile object")
	}
	dockerFile.DisableNoCache = !buildCtx.NoCache
	dockerFile.SkipHashLabel = !buildParams.allowLabels
	dockerFile.Context = buildCtx.GoContext()
	dockerFile.ContextPath = buildParams.dockerImage.Build.ContextPath
	dockerFile.Args, err = buildParams.dockerImage.Build.ArgsToMap()
	dockerFile.ReuseImagesWithSameCfg = buildParams.allowReuse
	if err != nil {
		return errors.Wrapf(err, "failed to convert docker args to map")
	}
	// docker build
	reader, err := dockerFile.Build()
	if err != nil {
		return errors.Wrapf(err, "failed to build docker image")
	}
	if err := reader.Listen(false, docker.MessageToLogFunc(buildCtx.Logger(), buildParams.subject)); err != nil {
		return err
	}

	if err := buildCtx.runAfterBuildScripts(root, module, buildParams, tags); err != nil {
		return errors.Wrapf(err, "failed to invoke scripts after push")
	}

	return nil
}

func (buildCtx *BuildContext) runInCustomImageContainer(action string, runID string, root *RootBuildDefinition, moduleName string, spec RunSpec) error {
	if spec.CustomImage.Name == "" {
		spec.CustomImage.Name = runID
	}
	tags, err := buildCtx.buildDockerImage(root, moduleName, dockerBuildParams{
		dockerImage: spec.CustomImage,
		subject:     action,
		allowReuse:  !buildCtx.NoCache,
		allowLabels: true,
	})
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return errors.Errorf("image build didn't return any tags for an image")
	}
	spec.Image = tags[0]

	run := runner.NewRun(buildCtx.CommonCtx)
	params, err := run.CalcRunInContainerParams(root.ProjectNameOrDefault(), root.ConfiguredRootPath(), spec.RunCfg, &root.Default.MainVolume)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate run params")
	}
	return run.RunInContainer(action, runID, params, spec)
}
