package build

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type Docker struct {
	CommonParams
	BuildParams

	DockerPush         bool
	DockerConfigPath   string
	KanikoExecutorPath string
	KanikoCachePath    string
	KanikoExtraArgs    string
	DockerImages       []string
}

func (o *Docker) Mount(a *kingpin.Application) *kingpin.CmdClause {
	_, rootDef, _ := types.ReadBuildModuleDefinition("")
	var dockerImageNames []string
	for _, module := range rootDef.Modules {
		for _, image := range rootDef.Default.DockerImages {
			dockerImageNames = util.AddIfNotExist(dockerImageNames, image.Name)
		}
		for _, profile := range rootDef.Profiles {
			for _, image := range profile.DockerImages {
				dockerImageNames = util.AddIfNotExist(dockerImageNames, image.Name)
			}
		}
		for _, image := range module.DockerImages {
			dockerImageNames = util.AddIfNotExist(dockerImageNames, image.Name)
		}
	}
	availableImages := strings.Join(dockerImageNames, "|")
	cmd := a.Command("docker", "Docker actions")
	o.registerCommonFlags(cmd)
	o.registerBuildFlags(cmd)
	buildCmd := cmd.Command("build", "Build Docker images specified for the project")
	buildCmd.Action(registerAction(o.Build))
	buildCmd.Flag("push", "Push after building (default: false)").
		BoolVar(&o.DockerPush)
	buildCmd.Arg("image", "Docker images to build ("+availableImages+")").
		StringsVar(&o.DockerImages)
	pushCmd := cmd.Command("push", "Push Docker images specified for the project")
	pushCmd.Action(registerAction(o.Push))
	pushCmd.Arg("image", "Docker images to push ("+availableImages+")").
		StringsVar(&o.DockerImages)
	configCmd := cmd.Command("effective-config", "Dumps effective Docker config.json (with auth data resolved)")
	configCmd.Arg("output-file", "Output file to dump effective Docker config.json to (e.g.: /path/to/config.json)").
		StringVar(&o.DockerConfigPath)
	configCmd.Action(registerAction(o.Config))
	kanikoCmd := cmd.Command("kaniko", "Build and push Docker images using Kaniko")
	kanikoCmd.Flag("executor-path", "Path to Kaniko executor binary (default: /kaniko/executor)").
		Short('E').
		Default("/kaniko/executor").
		StringVar(&o.KanikoExecutorPath)
	kanikoCmd.Flag("cache-path", "Path to Kaniko cache directory (default: /cache)").
		Short('C').
		Default("/cache").
		StringVar(&o.KanikoCachePath)
	kanikoCmd.Flag("extra-args", "Extra args to pass to kaniko executor").
		StringVar(&o.KanikoExtraArgs)
	kanikoCmd.Arg("image", "Docker images to build/push ("+availableImages+")").
		StringsVar(&o.DockerImages)
	kanikoCmd.Action(registerAction(o.Kaniko))
	appVersion = a.Model().Version

	return cmd
}

func (o *Docker) Push() error {
	buildCtx, err := o.ToBuildCtx("docker-push", o.CommonParams)
	if err != nil {
		return err
	}
	return buildCtx.PushDocker(o.DockerImages)
}

func (o *Docker) Build() error {
	buildCtx, err := o.ToBuildCtx("docker-build", o.CommonParams)
	if err != nil {
		return err
	}

	if err := buildCtx.BuildDocker(o.DockerImages); err != nil {
		return err
	} else if o.DockerPush {
		return o.Push()
	}
	return nil
}

func (o *Docker) Config() error {
	homeCfg, err := docker.ReadDockerConfigJson()
	if err != nil {
		return errors.Wrapf(err, "failed to read docker config")
	}

	err = homeCfg.ResolveExternalAuths()
	if err != nil {
		return errors.Wrapf(err, "failed to resolve external auth in docker config")
	}

	contBytes, err := json.Marshal(homeCfg)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal docker config")
	}
	if o.DockerConfigPath == "" {
		fmt.Println(string(contBytes))
	} else if err := ioutil.WriteFile(o.DockerConfigPath, contBytes, os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to write docker config.json to %q", o.DockerConfigPath)
	}
	return nil
}

func (o *Docker) Kaniko() error {
	buildCtx, err := o.ToBuildCtx("kaniko", o.CommonParams)
	if err != nil {
		return err
	}
	return buildCtx.KanikoBuild(welder.KanikoOpts{
		ExecutorPath: o.KanikoExecutorPath,
		CachePath:    o.KanikoCachePath,
		ExtraArgs:    o.KanikoExtraArgs,
		DockerImages: o.DockerImages,
	})
}
