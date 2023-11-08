package welder

import (
	"context"
	"fmt"
	"github.com/karrick/godirwalk"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/git"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type InitCtx struct {
	console      util.Console
	cwd          string
	buildCfgPath string
	preset       string
	Simple       bool
}

func NewInit(console util.Console, preset string) (*InitCtx, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect current dir")
	}
	return &InitCtx{
		console:      console,
		cwd:          cwd,
		buildCfgPath: path.Join(cwd, BuildConfigFileName),
		preset:       preset,
	}, nil
}

func (i *InitCtx) RunWizard() error {

	i.console.Writer().Println("Atlas CLI Build Wizard started")

	_, rootPath, _ := DetectBuildContext(i.cwd)
	if rootPath != "" {
		overwrite, err := i.console.AskYesNoQuestionWithDefault("Existing "+BuildConfigFileName+" file found. Are you sure you want to overwrite it? (Y/N)", true)
		if err != nil {
			return errors.Wrapf(err, "failed to read user response")
		}
		if !overwrite {
			i.console.Writer().Println("Wizard aborted")
			return nil
		}
	}

	buildCfgDef := RootBuildDefinition{SchemaVersion: RootBuildDefinitionSchemaVersion}

	if err := i.askProjectName(&buildCfgDef); err != nil {
		return err
	}

	if err := i.askProjectRoot(&buildCfgDef); err != nil {
		return err
	}

	if err := i.modifyGitIgnore(&buildCfgDef); err != nil {
		return err
	}

	return i.writeBuildCfgFile(&buildCfgDef)
	return nil
}

func (i *InitCtx) suggestDirsFrom(pattern string) ([]string, error) {
	matcher := regexp.MustCompile(fmt.Sprintf("(\\s|\\n|^||[^a-zA-Z0-9])+%s(\\s|\\n|$|[^a-zA-Z0-9])+", pattern))

	res := make([]string, 0)
	err := godirwalk.Walk(i.cwd, &godirwalk.Options{
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if matcher.Match([]byte(de.Name())) && de.IsDir() {
				relPath, err := filepath.Rel(i.cwd, osPathname)
				if err != nil {
					return errors.Wrapf(err, "failed to calculate relative path of %s from %s", osPathname, i.cwd)
				}
				res = append(res, relPath)
			}
			return nil
		},
		Unsorted: true,
	})
	return res, err
}

func (i *InitCtx) findDockerfiles(inPath string) ([]string, error) {
	res := make([]string, 0)
	i.console.Writer().Println("Scanning directory " + inPath + " for Dockerfile(s)...")
	dockerFileNameRegex := regexp.MustCompile("(?i).*Dockerfile")
	err := godirwalk.Walk(inPath, &godirwalk.Options{
		Callback: func(osPathname string, de *godirwalk.Dirent) error {
			if dockerFileNameRegex.Match([]byte(de.Name())) {
				dockerFile, err := docker.NewDockerfile(context.Background(), osPathname)
				if err != nil {
					return errors.Wrapf(err, "failed to read Dockerfile from %s", osPathname)
				}
				valid, err := dockerFile.IsValid()
				if err != nil {
					return errors.Wrapf(err, "failed to validate Dockerfile from %s", osPathname)
				}
				if valid {
					relPath, err := filepath.Rel(i.cwd, osPathname)
					if err != nil {
						return errors.Wrapf(err, "failed to calculate relative path of %s from %s", osPathname, i.cwd)
					}
					res = append(res, relPath)
				}
			}
			return nil
		},
	})
	if len(res) > 0 {
		i.console.Writer().Println("Found following Dockerfile(s) under ", inPath, ":")
	}
	for _, dFile := range res {
		i.console.Writer().Println("\t- ", dFile)
	}
	return res, err
}

func (i *InitCtx) writeBuildCfgFile(buildRootDef *RootBuildDefinition) error {
	i.console.Writer().Println("Writing ", i.buildCfgPath, "...")
	err := WriteYaml(buildRootDef, i.buildCfgPath)
	if err != nil {
		return errors.Wrapf(err, "failed to write build config file into %s", i.buildCfgPath)
	}
	return nil
}

func (i *InitCtx) askProjectName(buildRootDef *RootBuildDefinition) error {
	projectName, err := i.console.AskQuestionWithDefault("Specify project name", path.Base(i.cwd))
	if err != nil {
		return errors.Wrapf(err, "failed to read project sources root")
	}
	buildRootDef.ProjectName = projectName
	return nil
}

func (i *InitCtx) askProjectRoot(buildRootDef *RootBuildDefinition) error {
	projectRoot, err := i.console.AskQuestionWithDefault("Specify project sources root", ".")
	if err != nil {
		return errors.Wrapf(err, "failed to read project sources root")
	}
	buildRootDef.ProjectRoot = projectRoot
	return nil
}

func (i *InitCtx) modifyGitIgnore(buildRootDef *RootBuildDefinition) error {
	for {
		gitCtx, err := git.TraverseToRoot()
		if err != nil {
			i.console.Writer().Println("failed to detect Git root, is your project Git-based? ", err.Error())
			// no need to modify gitignore - not a git root
			return nil
		}
		gitRoot := gitCtx.Root()
		if !i.Simple {
			gitRoot, _ = i.console.AskQuestionWithDefault("Enter Git root for this project", gitRoot)

			_, err := os.Stat(filepath.Join(gitRoot, ".git"))

			if err != nil {
				i.console.Writer().Println(gitRoot + " is not a valid git path.")
				continue
			}
		}

		gitIgnorePath := filepath.Join(gitRoot, ".gitignore")
		_, err = os.Stat(gitIgnorePath)
		if !os.IsNotExist(err) {
			bytes, err := ioutil.ReadFile(gitIgnorePath)
			if err != nil {
				return errors.Wrapf(err, "failed to read .gitignore")
			}
			if strings.Contains(string(bytes), BuildOutputDir) {
				return nil
			}
		}

		f, err := os.OpenFile(gitIgnorePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)

		if err != nil {
			return errors.Wrapf(err, "failed to modify .gitignore")
		}

		_, err = f.WriteString("\n" + BuildOutputDir + "/**")

		return f.Close()
	}
}

func (i *InitCtx) configureModulesManually(root *RootBuildDefinition) error {
	for {
		more, _ := i.console.AskYesNoQuestionWithDefault("Would you like to add another module?", false)
		if !more {
			return nil
		}
		moduleName, err := i.console.AskQuestion("Enter module name")
		if err != nil {
			return errors.Wrapf(err, "failed to read module name")
		}
		moduleDef, err := i.configureModuleManually(root, moduleName)
		if err != nil {
			return errors.Wrapf(err, "failed to configure module")
		}
		root.Modules = append(root.Modules, moduleDef)
	}
}

func (i *InitCtx) configureModuleManually(root *RootBuildDefinition, moduleName string) (ModuleDefinition, error) {
	res := ModuleDefinition{Name: moduleName}

	possiblePaths, err := i.suggestDirsFrom(moduleName)
	if err != nil {
		return res, errors.Wrapf(err, "failed to suggest directory name")
	}
	possiblePath := ""
	if len(possiblePaths) > 0 {
		possiblePath = possiblePaths[0]
		i.console.Writer().Println("The following matching directories found:")
		for _, possiblePath := range possiblePaths {
			i.console.Writer().Println("\t- ", possiblePath)
		}
	}
	modulePath, err := i.console.AskQuestionWithDefault("Enter path to module "+moduleName, possiblePath)
	if err != nil {
		return res, errors.Wrapf(err, "failed to read module path for module %s", moduleName)
	}
	res.Path = modulePath

	return res, nil
}

func (i *InitCtx) addArgs(build *BuildDefinition) error {
	for {
		if len(build.Args) > 0 {
			i.console.Writer().Println("The following build args are defined at the moment: ")
			for k, v := range build.Args {
				i.console.Writer().Println("\t- ", k, "=", v)
			}
		}
		more, _ := i.console.AskYesNoQuestionWithDefault("Do you want to add more args?", false)
		if !more {
			return nil
		}
		argKV, err := i.console.AskQuestion("Enter argument definition in format <name>=<value>")
		if err != nil {
			return errors.Wrapf(err, "failed to read arg")
		}
		parts := strings.SplitN(argKV, "=", 1)
		build.Args[parts[0]] = StringValue(parts[1])
	}
}

func (i *InitCtx) addVolumes(build *BuildDefinition) error {
	for {
		if len(build.Volumes) > 0 {
			i.console.Writer().Println("The following volumes are defined at the moment: ")
			for _, v := range build.Volumes {
				i.console.Writer().Println("\t- ", v)
			}
		}
		more, _ := i.console.AskYesNoQuestionWithDefault("Do you want to add more volumes?", false)
		if !more {
			return nil
		}
		volume, err := i.console.AskQuestion("Enter volume definition in format <hostPath>:<containerPath>:<(ro|rw)>")
		if err != nil {
			return errors.Wrapf(err, "failed to read volume")
		}
		build.Volumes = append(build.Volumes, volume)
	}
}

func (i *InitCtx) addEnv(build *BuildDefinition) error {
	for {
		if len(build.Env) > 0 {
			i.console.Writer().Println("The following env variables are defined: ")
			for k, v := range build.Env {
				i.console.Writer().Println("\t- ", k, "=", v)
			}
		}
		more, _ := i.console.AskYesNoQuestionWithDefault("Do you want to add more env variables?", false)
		if !more {
			return nil
		}
		argKV, err := i.console.AskQuestion("Enter env variable definition in format <name>=<value>")
		if err != nil {
			return errors.Wrapf(err, "failed to read env")
		}
		parts := strings.SplitN(argKV, "=", 1)
		build.Env[parts[0]] = StringValue(parts[1])
	}

}
