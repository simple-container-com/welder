package types

import (
	"fmt"
	"github.com/Masterminds/semver/v3"
	ghodss_yaml "github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	BuildConfigFileName  = "welder.yaml"
	BuildOutputDir       = ".welder-out"
	OutDockerFileName    = "docker.yaml"
	OutDockerEnvFileName = "docker-pushed-images.sh"
)

func readYaml(pathToYaml string) []byte {
	filename, err := filepath.Abs(pathToYaml)
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		handleErr(errors.Wrapf(err, "failed to unmarshal file %s", pathToYaml))
	}
	return yamlFile
}

// DetectBuildContext finds build root by traversing up to welder.yaml
func DetectBuildContext(rootDir string) (string, string, error) {
	var cwd string
	if rootDir == "" {
		getCwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		rootDir = getCwd
		cwd = getCwd
	} else {
		cwd = rootDir
	}
	_, err := os.Stat(filepath.Join(rootDir, BuildConfigFileName))
	for os.IsNotExist(err) && filepath.Dir(rootDir) != "/" {
		rootDir = filepath.Dir(rootDir)
		_, err = os.Stat(filepath.Join(rootDir, BuildConfigFileName))
	}
	if filepath.Dir(rootDir) == "/" {
		return "", "", fmt.Errorf("could not determine project root, make sure you're in the project context. Current dir: %s", cwd)
	}
	relPath, err := filepath.Rel(rootDir, cwd)
	if err != nil {
		return "", "", err
	}
	rootDir, err = filepath.Abs(rootDir)
	if err != nil {
		return "", "", err
	}
	return rootDir, relPath, nil
}

// ReadBuildModuleDefinition contextually reads current module definition
// if executed from the project root returns nil as module definition
func ReadBuildModuleDefinition(rootDir string) (*ModuleDefinition, RootBuildDefinition, error) {
	rootDir, subPath, err := DetectBuildContext(rootDir)
	if err != nil {
		return nil, RootBuildDefinition{}, err
	}

	rootDef, err := ReadBuildRootDefinition(rootDir)
	if err != nil {
		return nil, rootDef, err
	}
	rootDef.ProjectRoot = path.Join(rootDir, rootDef.ProjectRoot)
	detectedModules := make([]ModuleDefinition, 0)
	for _, module := range rootDef.Modules {
		modulePath, err := filepath.Abs(filepath.Join(rootDir, module.Path))
		if err != nil {
			return nil, rootDef, err
		}
		relPath, err := filepath.Abs(filepath.Join(rootDir, subPath))
		if err != nil {
			return nil, rootDef, err
		}
		if modulePath == relPath {
			detectedModules = append(detectedModules, module)
		}
	}
	if len(detectedModules) == 1 {
		return &detectedModules[0], rootDef, nil
	}
	return nil, rootDef, nil
}

func checkSupportedSchemaVersion(yamlFilePath string, yamlContent []byte) error {
	var vd VersionedDefinition
	err := yaml.Unmarshal(yamlContent, &vd)
	if err != nil {
		return err
	}

	if loadingVersion, err := semver.NewVersion(vd.SchemaVersion); err != nil {
		return errors.Wrapf(err, "failed to parse schema version: %q", vd.SchemaVersion)
	} else if currentVersion, err := semver.NewVersion(RootBuildDefinitionSchemaVersion); err != nil {
		return errors.Wrapf(err, "failed to parse roob build schema version: %q", currentVersion)
	} else if loadingVersion.GreaterThan(currentVersion) {
		return errors.Errorf("your file %s is made for more recent version of "+
			"welder (%s), current version: %s", yamlFilePath, vd.SchemaVersion, RootBuildDefinitionSchemaVersion)
	}

	return nil
}

// ReadBuildRootDefinition reads root definition from specified directory
func ReadBuildRootDefinition(basePath string) (RootBuildDefinition, error) {
	yamlFilePath := filepath.Join(basePath, BuildConfigFileName)
	yamlFile := readYaml(yamlFilePath)

	if err := checkSupportedSchemaVersion(yamlFilePath, yamlFile); err != nil {
		return RootBuildDefinition{}, err
	}

	var rb RootBuildDefinition
	err := yaml.UnmarshalStrict(yamlFile, &rb)
	if err != nil {
		return rb, err
	}

	rb.rootDir = basePath
	rb.initCaches()
	return rb, nil
}

func (build *OutDockerDefinition) WriteToOutputDir(rootDir string) error {
	rootDir, _, err := DetectBuildContext(rootDir)
	if err != nil {
		return errors.Wrapf(err, "failed to detect build context")
	}
	outputDir := path.Join(rootDir, BuildOutputDir)
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "failed to create output directory")
	}

	outFilePath := path.Join(rootDir, BuildOutputDir, OutDockerFileName)
	if err := WriteYaml(build, outFilePath); err != nil {
		return errors.Wrap(err, "failed to write to docker.yaml")
	}
	var envFileContent strings.Builder
	for _, module := range build.Modules {
		for _, image := range module.DockerImages {
			for i, digest := range image.Digests {
				prefix := fmt.Sprint(fmt.Sprintf("export %s_%s_",
					strings.ReplaceAll(strings.ToUpper(module.Name), "-", "_"),
					strings.ReplaceAll(strings.ToUpper(image.Name), "-", "_"),
				))
				if i == 0 {
					envFileContent.WriteString(prefix + "TAG=" + digest.Tag + "\n")
					envFileContent.WriteString(prefix + "IMAGE=" + digest.Image + "\n")
					envFileContent.WriteString(prefix + "DIGEST=" + digest.Digest + "\n")
				}
				envFileContent.WriteString(prefix + strconv.Itoa(i) + "_TAG=" + digest.Tag + "\n")
				envFileContent.WriteString(prefix + strconv.Itoa(i) + "_IMAGE=" + digest.Image + "\n")
				envFileContent.WriteString(prefix + strconv.Itoa(i) + "_DIGEST=" + digest.Digest + "\n")
			}
		}
	}
	envFilePath := path.Join(rootDir, BuildOutputDir, OutDockerEnvFileName)
	return ioutil.WriteFile(envFilePath, []byte(envFileContent.String()), 0644)
}

func handleErr(err error) {
	if err != nil {
		panic(errors.Wrap(err, "handling error"))
	}
}

func WriteYaml(data interface{}, file string) error {
	yamlFile, err := yaml.Marshal(data)

	if err != nil {
		return err
	}

	return ioutil.WriteFile(file, yamlFile, 0644)
}

func ReadOutDockerDefinition(pathToFile string) (OutDockerDefinition, error) {
	yamlFile := readYaml(pathToFile)
	var res OutDockerDefinition
	err := ghodss_yaml.Unmarshal(yamlFile, &res)
	return res, err
}

func WriteModulePointerYaml(pointer DeployModulePointer, file string) error {
	return WriteYaml(pointer, file)
}
