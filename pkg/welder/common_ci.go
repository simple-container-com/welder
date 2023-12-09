package welder

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"sort"
	"text/template"

	"github.com/pkg/errors"

	"github.com/simple-container-com/welder/pkg/git"
	"github.com/simple-container-com/welder/pkg/render/rendered"
	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

// CIModule describes a module for CI generator
type CIModule struct {
	Key            string
	Dir            string
	Name           string
	DoDocker       bool
	DoDeploy       bool
	DoMicrosDeploy bool
}

// CILocation holds info about deployment target
type CILocation struct {
	Name         string
	AutoDeployed bool
}

// CIModuleLocations struct representing set of available deployment targets for a module
type CIModuleLocations struct {
	ModuleName string
	Module     CIModule
	Locations  []CILocation
}

// CommonCIGenerator struct holding common properties for CI config generators
type CommonCIGenerator struct {
	RootPath      string
	TargetDirPath string

	// autofilled
	HasDeployments            bool
	HasDockerImages           bool
	Locations                 []CIModuleLocations
	AutoDeployedLocations     []CIModuleLocations
	ManuallyDeployedLocations []CIModuleLocations
	BuildOutputDirName        string
	Modules                   []CIModule
	ServiceName               string
	InitFiles                 map[string]string
	AssetsBaseDir             string

	tpl *template.Template
}

func (o *CommonCIGenerator) Init() (RootBuildDefinition, error) {
	_, rootDef, err := ReadBuildModuleDefinition(o.RootPath)
	if err != nil {
		return RootBuildDefinition{}, errors.Wrap(err, "could not find project configuration in the root path defined")
	}

	o.TargetDirPath = o.RootPath
	o.BuildOutputDirName = BuildOutputDir
	o.Modules = make([]CIModule, len(rootDef.Modules))
	o.Locations = make([]CIModuleLocations, 0)

	o.HasDeployments = false
	o.HasDockerImages = false
	o.AutoDeployedLocations = make([]CIModuleLocations, 0)
	o.ManuallyDeployedLocations = make([]CIModuleLocations, 0)
	for i, module := range rootDef.Modules {
		o.Modules[i] = CIModule{
			Name: module.Name,
			Dir:  module.Path,
			Key:  toModuleKey(module.Name),
		}
		var gitClient git.Git
		if gitCl, err := git.TraverseToRoot(); err == nil {
			gitClient = gitCl
		}
		commonCtx := NewCommonContext(&CommonCtx{}, &util.NoopLogger{})
		commonCtx.SetGitClient(gitClient)
		buildCtx := NewBuildContext(&BuildContext{CommonCtx: commonCtx}, &util.NoopLogger{})

		deploy, _, err := buildCtx.ActualDeployDefinitionFor(&rootDef, module.Name, nil)
		if err != nil {
			return rootDef, errors.Wrapf(err, "failed to calculate deploy definition for module %s", module.Name)
		}

		dockerImages, err := buildCtx.ActualDockerImagesDefinitionFor(&rootDef, module.Name)
		if err != nil {
			return rootDef, errors.Wrapf(err, "failed to calculate docker definition for module %s", module.Name)
		}

		moduleLocations := make([]CILocation, 0)
		environments := make(DeployEnvsDefinition, 0)
		if len(deploy.Environments) > 0 {
			environments = deploy.Environments
			o.Modules[i].DoDeploy = true
		}
		if len(environments) > 0 {
			moduleLocations = o.appendLocations(moduleLocations, environments)
			o.HasDeployments = true
		}

		doDocker := len(dockerImages) > 0
		o.Modules[i].DoDocker = doDocker
		o.HasDockerImages = o.HasDockerImages || doDocker
		locations := CIModuleLocations{
			ModuleName: module.Name,
			Module:     o.Modules[i],
			Locations:  moduleLocations,
		}
		o.Locations = append(o.Locations, locations)
		autoDeployed := make([]CILocation, 0)
		manuallyDeployed := make([]CILocation, 0)
		for name, env := range environments {
			if env.AutoDeploy {
				autoDeployed = append(autoDeployed, CILocation{
					Name: name, AutoDeployed: true,
				})
			} else {
				manuallyDeployed = append(manuallyDeployed, CILocation{
					Name: name,
				})
			}
		}
		if len(autoDeployed) > 0 {
			o.AutoDeployedLocations = append(o.AutoDeployedLocations, CIModuleLocations{
				ModuleName: module.Name,
				Module:     o.Modules[i],
				Locations:  autoDeployed,
			})
		}
		if len(manuallyDeployed) > 0 {
			o.ManuallyDeployedLocations = append(o.ManuallyDeployedLocations, CIModuleLocations{
				ModuleName: module.Name,
				Module:     o.Modules[i],
				Locations:  manuallyDeployed,
			})
		}
	}

	// Sort locations in each module
	for locIdx := range o.Locations {
		module := o.Locations[locIdx]
		sort.SliceStable(o.Locations[locIdx].Locations, func(i, j int) bool {
			return module.Locations[i].Name < module.Locations[j].Name
		})
	}

	// Sort modules in bamboo locations list
	sort.SliceStable(o.Locations, func(i, j int) bool {
		return o.Locations[i].ModuleName < o.Locations[j].ModuleName
	})

	o.tpl = template.New(o.RootPath).Funcs(template.FuncMap{
		"plus1": func(x int) int {
			return x + 1
		},
	})

	return rootDef, nil
}

func (o *CommonCIGenerator) Generate(context interface{}) error {
	for sourceFile, targetFile := range o.InitFiles {
		fmt.Print(" > Processing " + sourceFile + "...")
		data, err := rendered.Asset(o.AssetsBaseDir + sourceFile)
		if err != nil {
			return errors.Wrapf(err, "failed to get asset '%s' from '%s'", sourceFile, o.AssetsBaseDir)
		}
		targetFilePath := path.Join(o.TargetDirPath, targetFile)
		var processedTmpl bytes.Buffer
		if tmpl, err := o.tpl.Parse(string(data)); err != nil {
			return errors.Wrapf(err, "failed to parse template for '%s'", sourceFile)
		} else if err := tmpl.Execute(&processedTmpl, context); err != nil {
			return errors.Wrapf(err, "failed to execute template for '%s'", sourceFile)
		} else if err := os.MkdirAll(path.Dir(targetFilePath), os.ModePerm); err != nil {
			return errors.Wrapf(err, "failed to create directory '%s'", o.TargetDirPath)
		} else if err := os.WriteFile(targetFilePath, processedTmpl.Bytes(), os.ModePerm); err != nil {
			return errors.Wrapf(err, "failed to write file '%s'", targetFilePath)
		}
		fmt.Println("DONE")
	}
	return nil
}
