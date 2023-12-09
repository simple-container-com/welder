package welder

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/pkg/errors"

	. "github.com/simple-container-com/welder/pkg/welder/types"
)

type BambooSpecs struct {
	CommonCIGenerator

	// required
	DirectoryName  string
	PackageName    string
	LinkedRepoName string
	// optional
	ProjectKey     string
	GitRepoUrl     string
	Owner          string
	BuildPbcImage  string
	DeployPbcImage string
	SlackChannel   string
}

func (o *BambooSpecs) Init() error {
	return nil
}

func (o *BambooSpecs) Generate() error {
	rootDef, err := o.CommonCIGenerator.Init()
	if err != nil {
		return errors.Wrapf(err, "failed to init template")
	}

	if len(o.InitFiles) == 0 {
		o.InitFiles = map[string]string{
			".gitignore":         ".gitignore",
			"pom.xml.tpl":        "pom.xml",
			"BambooSpecs.kt.tpl": path.Join("src/main/kotlin", packageToDir(o.PackageName), "BambooSpecs.kt"),
		}
	}

	if len(o.AssetsBaseDir) == 0 {
		o.AssetsBaseDir = "bamboo/specs/"
	}

	specsDir := path.Join(o.RootPath, o.DirectoryName)
	if err := os.MkdirAll(specsDir, os.ModePerm); err != nil {
		return err
	}

	o.TargetDirPath = specsDir

	if o.ProjectKey == "" {
		o.ProjectKey = toProjectKey(rootDef.ProjectName)
	}

	if o.PackageName == "" {
		o.PackageName = "com.atlassian"
	}

	if o.Owner == "" {
		usr, err := user.Current()
		if err == nil {
			o.Owner = usr.Username
		} else {
			return errors.Wrap(err, "failed to autodetect owner")
		}
	}

	if o.LinkedRepoName == "" {
		o.LinkedRepoName = rootDef.ProjectName
	}

	if o.BuildPbcImage == "" {
		o.BuildPbcImage = "docker.simple-container.com/sox/deng/welder-pbc:latest"
	}

	if o.DeployPbcImage == "" {
		o.DeployPbcImage = "docker.simple-container.com/sox/deng/welder-pbc:latest"
	}

	if err := o.CommonCIGenerator.Generate(o); err != nil {
		return errors.Wrapf(err, "failed to generate build system files")
	}

	fmt.Println("BambooSpecs module has been successfully generated in " + specsDir)

	return nil
}

func (o *CommonCIGenerator) appendLocations(locations []CILocation, envs DeployEnvsDefinition) []CILocation {
	for name, env := range envs {
		found := false
		for _, existingValue := range locations {
			if existingValue.Name == name {
				found = true
			}
		}
		if !found {
			locations = append(locations, CILocation{
				Name:         name,
				AutoDeployed: env.AutoDeploy,
			})
		}
	}
	return locations
}

func (o *CommonCIGenerator) sanitizeBambooEnvVar(key string) string {
	return strings.Replace(strings.ToLower(key), "[^A-Za-z0-9]", "", -1)
}

func packageToDir(packageName string) string {
	return strings.Replace(packageName, ".", "/", -1)
}

func toProjectKey(serviceName string) string {
	replacer := strings.NewReplacer(" ", "", "-", "")
	return strings.ToUpper(replacer.Replace(serviceName))
}

func toModuleKey(moduleName string) string {
	replacer := strings.NewReplacer(" ", "", "-", "")
	return strings.ToUpper(replacer.Replace(moduleName))
}
