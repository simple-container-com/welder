//go:build mage
// +build mage

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/atombender/go-jsonschema/pkg/generator"
	_ "github.com/go-bindata/go-bindata" // generator
	"github.com/go-bindata/go-bindata/v3"
	"github.com/invopop/jsonschema"
	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/magebuild"
	"github.com/simple-container-com/welder/pkg/welder/types"
	"golang.org/x/sync/errgroup"
	/**/)

// Default target to run when none is specified
// If not set, running mage will list available targets
// var Default = Build

type (
	Build    mg.Namespace
	Tests    mg.Namespace
	Generate mg.Namespace
	Git      mg.Namespace
	Publish  mg.Namespace
)

var (
	Ctx        = magebuild.InitBuild()
	Generators = map[string]magebuild.Generator{
		"templates":  nsGenerate.Templates,
		"gofmt":      nsGenerate.Formatting,
		"jsonschema": nsGenerate.JsonSchema,
		//"pipelines-schema": nsGenerate.PipelinesSchema, // TODO
	}
)

var (
	nsGenerate = Generate{}
	nsBuild    = Build{}
	nsTests    = Tests{}
	nsGit      = Git{}
	nsPublish  = Publish{}
)

// --------------------------------------
// Git targets

// CheckStatus Checks current status and fails if there are changes
func (Git) CheckStatus() error {
	return Ctx.GitCheckWorkTree()
}

// --------------------------------------
// Test targets

// Test Runs all types of tests
func Test() error {
	Ctx.JUnitOutputFile = "bin/test-results/TEST-junit-report-unit.xml"
	return Ctx.Test(Ctx.TestTarget, fmt.Sprintf("-tags='%s'", Ctx.TestTags))
}

// --------------------------------------
// Build targets

// All Run everything: install deps, generate clients, run all builds for all platforms
func (b Build) All() error {
	if err := b.Build(); err != nil {
		return err
	}
	if err := nsGit.CheckStatus(); err != nil {
		return err
	}
	return b.Bundles()
}

// Build Builds all plugins at once
func (b Build) Build() error {
	if err := b.Deps(); err != nil {
		return err
	}
	if err := nsGenerate.All(); err != nil {
		return err
	}
	if Ctx.BuildViaWelder {
		return Ctx.Welder().Build()
	}
	if err := Ctx.ForAllTargets(func(target magebuild.Target) error {
		return Ctx.ForAllPlatforms(func(platform magebuild.Platform) error {
			var extraArgs []string
			if platform.GOOS != "windows" {
				extraArgs = []string{"-tags=osusergo"}
			}
			if err := Ctx.Build(target, platform, extraArgs...); err != nil {
				return errors.Wrapf(err, "failed to build %s: check bin/test-results for test report", target)
			}
			return nil
		})
	}); err != nil {
		return err
	}
	return Test()
}

// Deps Cleans up output directory
func (Build) Deps() error {
	fmt.Println("Installing dependencies...")
	return Ctx.RunCmd(magebuild.Cmd{Command: "go mod download", Env: Ctx.CurrentPlatform().GoEnv()})
}

// Clean Cleans up output directory
func (Build) Clean() error {
	fmt.Println("Cleaning...")
	return os.RemoveAll(Ctx.OutDir)
}

// Bundles Build tarballs out of binaries
func (b Build) Bundles() error {
	fmt.Println("Bundling tarballs...")
	return Ctx.ForAllTargets(func(target magebuild.Target) error {
		return Ctx.ForAllPlatforms(func(platform magebuild.Platform) error {
			return Ctx.BuildBundle(Ctx.Bundle(target, platform))
		})
	})
}

// --------------------------------------
// Publish

// Manifest Publishes updated manifest to statlas
func (pub Publish) Manifest() error {
	return Ctx.ForAllTargets(func(target magebuild.Target) error {
		return Ctx.PublishNewRelease(target, Ctx.GetVersion())
	})
}

// All Publishes tarballs and updated manifest to statlas
func (pub Publish) All() error {
	if err := Ctx.ForAllPlatforms(func(platform magebuild.Platform) error {
		return Ctx.ForAllTargets(func(target magebuild.Target) error {
			return pub.publishTarget(target, platform)
		})
	}); err != nil {
		return err
	}
	if err := pub.Manifest(); err != nil {
		return err
	}
	return nil
}

func (Publish) publishTarget(target magebuild.Target, platform magebuild.Platform) error {
	var eg errgroup.Group
	t := target
	p := platform
	bundle := Ctx.Bundle(t, p)
	publishBundle := func() error {
		if err := Ctx.Publish(bundle, Ctx.GetVersion()); err != nil {
			return err
		}
		return Ctx.Publish(bundle, "latest")
	}
	if Ctx.IsParallel() {
		eg.Go(publishBundle)
	} else if err := publishBundle(); err != nil {
		return err
	}
	return eg.Wait()
}

// --------------------------------------
// Generators

// Invoke all generators and generate all necessary clients
func (Generate) All() error {
	var eg errgroup.Group
	for _, generator := range Generators {
		g := generator
		if Ctx.IsParallel() {
			eg.Go(g)
		} else if err := g(); err != nil {
			fmt.Println(err.Error())
			return errors.Wrapf(err, "Failed to execute generator")
		}
	}
	return eg.Wait()
}

// Templates Generate templates.tpl.go file from /templates folder
func (Generate) Templates() error {
	fmt.Println("Regenerating templates.tpl.go...")
	templatesDir := Ctx.Path("templates")
	if paths, err := Ctx.ListAllSubDirs(templatesDir); err != nil {
		return err
	} else {
		fmt.Println(fmt.Sprintf("Generating for paths: %s", strings.Join(paths, ",")))
		config := bindata.NewConfig()
		config.Package = "rendered"
		config.Prefix = fmt.Sprintf("%s/", templatesDir)
		config.Output = Ctx.Path("pkg/render/rendered/templates.tpl.go")
		config.Input = []bindata.InputConfig{}
		config.NoMetadata = true
		config.Ignore = []*regexp.Regexp{regexp.MustCompile("\\.DS_Store")}
		for _, path := range paths {
			config.Input = append(config.Input, bindata.InputConfig{Path: fmt.Sprintf("%s/%s", templatesDir, path)})
		}
		return bindata.Translate(config)
	}
}

// Formatting runs gofmt
func (Generate) Formatting() error {
	fmt.Println("Reformat code")
	err := Ctx.RunCmd(magebuild.Cmd{Command: "go fmt $(go list ./... | grep -v render/rendered)", Env: Ctx.CurrentPlatform().GoEnv()})
	if err != nil {
		return err
	}
	return Ctx.RunCmd(magebuild.Cmd{Command: "go run mvdan.cc/gofumpt -l -w .", Env: Ctx.CurrentPlatform().GoEnv()})
}

// JsonSchema generate JSON schema to use within the IntelliJ IDEA plugin
func (Generate) JsonSchema() error {
	schema := jsonschema.Reflect(&types.RootBuildDefinition{})
	contBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSONSchema")
	}
	if err := os.MkdirAll(Ctx.Path("bin"), os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to create bin output directory")
	}
	path := Ctx.Path("bin/welder.schema.json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s for writing", path)
	}
	_, err = f.WriteString(string(contBytes))
	return err
}

// PipelinesSchema generate structures based on the bitbucket-pipelines.schema.json file
func (Generate) PipelinesSchema() error {
	cfg := generator.Config{
		Warner: func(message string) {
			fmt.Println(fmt.Sprintf("Warning: %s", message))
		},
		DefaultPackageName: "schema",
		DefaultOutputName:  Ctx.Path("pkg/pipelines/schema/structs.go"),
		SchemaMappings: []generator.SchemaMapping{
			{RootType: "BitbucketPipelines", PackageName: "schema", SchemaID: "https://bitbucket.org/pipelines.json"},
		},
	}

	gen, err := generator.New(cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to create generator")
	}

	pipelinesJsonSchemaFile := Ctx.Path("pkg/pipelines/schema/bitbucket-pipelines.schema.json")
	err = gen.DoFile(pipelinesJsonSchemaFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read file %s", pipelinesJsonSchemaFile)
	}
	for path, contBytes := range gen.Sources() {
		fmt.Println(fmt.Sprintf("Generating %s...", path))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s for writing", path)
		}

		if err != nil {
			return errors.Wrapf(err, "failed to create output file for pipelines schema")
		}

		_, err = f.WriteString(string(contBytes))
		if err != nil {
			return errors.Wrapf(err, "failed to write to file %s", path)
		}
	}
	return nil
}
