package welder

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/mitchellh/go-homedir"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/git/mock"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strings"
	"testing"
	"time"
)

func TestDockerBuild(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()

	logger := util.NewPipeLogger()

	var eg errgroup.Group
	eg.Go(func() error {
		var buffer bytes.Buffer
		bufReader := bufio.NewReader(logger.Reader())
		for {
			outBytes, _, err := bufReader.ReadLine()
			buffer.Write(outBytes)
			buffer.Write([]byte("\n"))
			switch err {
			case io.EOF:
				output := string(buffer.String())
				Expect(output).To(ContainSubstring("val: module=armory"))
				Expect(output).To(ContainSubstring("[trebuchet] [service]          OK new line"))
				return nil
			case nil:
				fmt.Println(string(outBytes))
			default:
				return errors.Wrapf(err, "failed to read next line from build output")
			}
		}
	})

	buildCtx := NewBuildContext(&BuildContext{
		CommonCtx: &CommonCtx{
			NoCache: true,
		},
	}, logger)
	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f-test", nil)
	gitMock.On("Branch").Return("feature/test", nil)
	buildCtx.SetGitClient(&gitMock)
	buildCtx.SetRootDir(projectDir)

	_, root, err := ReadBuildModuleDefinition(projectDir)
	dockerImages, err := buildCtx.ActualDockerImagesDefinitionFor(&root, "armory")

	Expect(err).To(BeNil())
	fmt.Println(dockerImages[0].Tags[0])

	Expect(buildCtx.BuildDocker([]string{})).To(BeNil())
	Expect(logger.Close()).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}

func TestRunTasks(t *testing.T) {
	RegisterTestingT(t)

	curUser, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()

	logger := util.NewPipeLogger()

	var eg errgroup.Group
	eg.Go(func() error {
		var buffer bytes.Buffer
		bufReader := bufio.NewReader(logger.Reader())
		for {
			outBytes, _, err := bufReader.ReadLine()
			buffer.Write(outBytes)
			buffer.Write([]byte("\n"))
			switch err {
			case io.EOF:
				output := buffer.String()
				homeDir := fmt.Sprintf("/home/%s", curUser.Username)
				if curUser.Username == "root" {
					homeDir = "/root"
				}
				Expect(output).To(ContainSubstring("[print-home-dir] home=" + homeDir))
				Expect(output).To(ContainSubstring("[print-home-dir]     <modelVersion>4.0.0</modelVersion>"))
				return nil
			case nil:
				fmt.Println(string(outBytes))
			default:
				return errors.Wrapf(err, "failed to read next line from build output")
			}
		}
	})
	buildCtx := NewBuildContext(&BuildContext{&CommonCtx{Username: curUser.Username, Verbose: true}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Run("print-home-dir", 0, "print-home-dir")).To(BeNil())
	Expect(logger.Close()).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}

func TestModuleBuild(t *testing.T) {
	RegisterTestingT(t)

	curUser, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()

	logger := util.NewPipeLogger()
	var eg errgroup.Group
	eg.Go(func() error {
		var buffer bytes.Buffer
		scanner := util.NewLineOrReturnScanner(logger.Reader())
		for {
			if !scanner.Scan() {
				if scanner.Err() != nil {
					return errors.Wrapf(scanner.Err(), "failed to read next line from build output")
				}
				break
			}
			switch scanner.Err() {
			case nil:
				buffer.Write([]byte(scanner.Text()))
				buffer.Write([]byte("\n"))
				fmt.Println(time.Now().Format("15:04:05> ") + scanner.Text())
			default:
				return errors.Wrapf(scanner.Err(), "failed to read next line from build output")
			}
		}
		output := buffer.String()
		Expect(output).To(ContainSubstring("[trebuchet] [0] BUILD_SOMETHING=BUILDVAL"))
		Expect(output).To(ContainSubstring("[trebuchet] [0] ENV_WITH_DEFAULT_ARG=defaultEnvValue"))
		Expect(output).To(ContainSubstring("[trebuchet] [0] ENV_FROM_ARG_WITH_DEFAULT=defaultArgValue"))
		Expect(output).To(ContainSubstring("[armory] [0] BUILD_SOMETHING=BUILDVAL"))
		Expect(output).To(ContainSubstring("[armory] [0] TEST_SOMETHING=TESTVAL"))
		Expect(output).NotTo(ContainSubstring("[trebuchet] [0] TEST_SOMETHING=TESTVAL"))
		Expect(output).To(ContainSubstring("[trebuchet] [0] BUILD_ARGS= -DskipTests"))
		Expect(output).To(ContainSubstring(fmt.Sprintf("[armory] [0] %s", curUser.Username)))
		Expect(output).To(ContainSubstring("[armory] [0] [INFO] Building jar: /some/directory/armory/target/armory-1.0-SNAPSHOT.jar"))
		Expect(output).To(ContainSubstring(fmt.Sprintf("[trebuchet] [0] [INFO] Building jar: %s/services/trebuchet/target/trebuchet-1.0-SNAPSHOT.jar", projectDir)))
		Expect(output).To(ContainSubstring("[trebuchet] [0] =======> 7"))
		Expect(output).To(ContainSubstring("[trebuchet] [do-echo] -Dsome.other.var=blah something"))
		Expect(output).To(ContainSubstring("[trebuchet] [2] i-am-on-host"))
		outPath := path.Join(projectDir, "services", "armory", "target", "armory-1.0-SNAPSHOT.jar")
		_, err := os.Stat(outPath)
		Expect(os.IsNotExist(err)).To(Equal(false), "file "+outPath+" must exist")
		return nil
	})
	ensureMavenEnvExists()
	buildCtx := NewBuildContext(&BuildContext{
		CommonCtx: &CommonCtx{
			Modules:  []string{"armory", "trebuchet"},
			Username: curUser.Username,
			Profiles: []string{"skip-tests"},
			Verbose:  true,
		},
	}, logger)
	buildCtx.SetRootDir(projectDir)

	os.Setenv("BUILD_SOMETHING", "BUILDVAL")
	os.Setenv("TEST_SOMETHING", "TESTVAL")
	defer os.Unsetenv("BUILD_SOMETHING")
	defer os.Unsetenv("TEST_SOMETHING")

	Expect(buildCtx.Build()).To(BeNil())
	Expect(logger.Close()).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}

func TestModuleDeployDdev(t *testing.T) {
	RegisterTestingT(t)

	curUser, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()

	deployContext := NewDeployContext(NewBuildContext(&BuildContext{
		CommonCtx: &CommonCtx{
			Modules: []string{"third"}, SoxEnabled: true,
			Verbose: true, Username: curUser.Username,
		},
	}, util.NewStdoutLogger(os.Stdout, os.Stderr).Debug()), []string{"ddev"})
	deployContext.SetRootDir(projectDir)

	Expect(deployContext.Deploy()).To(BeNil())

	deployVerFile := path.Join(projectDir, "services", "armory", "target", "deploy-version")
	deployFlagsFile := path.Join(projectDir, "services", "armory", "target", "deploy-flags")
	deployEnvFile := path.Join(projectDir, "services", "armory", "target", "deploy-env")
	hostTaskFile := path.Join(projectDir, "services", "armory", "target", "host-task")
	_, err := os.Stat(deployVerFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+deployEnvFile+" must exist")
	_, err = os.Stat(deployVerFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+deployVerFile+" must exist")
	_, err = os.Stat(deployFlagsFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+deployFlagsFile+" must exist")
	_, err = os.Stat(hostTaskFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+hostTaskFile+" must exist")
	deployVerBytes, _ := ioutil.ReadFile(deployVerFile)
	deployFlagsBytes, _ := ioutil.ReadFile(deployFlagsFile)
	deployEnvBytes, _ := ioutil.ReadFile(deployEnvFile)
	hostTaskBytes, _ := ioutil.ReadFile(hostTaskFile)
	Expect(strings.TrimSpace(string(deployVerBytes))).To(Equal("1.0.0-beta"))
	Expect(strings.TrimSpace(string(deployFlagsBytes))).To(Equal("--strict ddev"))
	Expect(strings.TrimSpace(string(deployEnvBytes))).To(Equal("ddev=dev\nSOME_DEPLOY_VAR=value"))
	Expect(strings.TrimSpace(string(hostTaskBytes))).To(Equal("-Dsome.host.arg=yay-host -Dproject.version=1.0.0-beta extra-arg-host"))
}

func TestModuleDeployStg(t *testing.T) {
	RegisterTestingT(t)

	curUser, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()
	deployContext := NewDeployContext(NewBuildContext(&BuildContext{
		CommonCtx: &CommonCtx{
			Modules: []string{"third"}, SoxEnabled: true,
			Verbose: true, Username: curUser.Username,
		},
	}, util.NewStdoutLogger(os.Stdout, os.Stderr)), []string{"stg-west"})
	deployContext.SetRootDir(projectDir)

	Expect(deployContext.Deploy()).To(BeNil())
	deployEnvFile := path.Join(projectDir, "services", "armory", "target", "deploy-env")
	_, err := os.Stat(deployEnvFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+deployEnvFile+" must exist")
	deployEnvBytes, _ := ioutil.ReadFile(deployEnvFile)
	Expect(strings.TrimSpace(string(deployEnvBytes))).To(Equal("stg-west=staging\nSOME_DEPLOY_VAR=staging-value"))
}

func TestWritingDockerOutputFile(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/example")
	defer cleanup()

	dockerDefs := OutDockerDefinition{
		Modules: []OutDockerModuleDefinition{
			{
				Name: "module-with-dash",
				DockerImages: []OutDockerImageDefinition{
					{
						Name: "image-with-dash",
						Digests: []OutDockerDigestDefinition{
							{
								Tag:    "tag1",
								Digest: "digest1",
								Image:  "image1",
							},
							{
								Tag:    "tag2",
								Digest: "digest2",
								Image:  "image2",
							},
						},
					},
				},
			},
		},
	}
	Expect(dockerDefs.WriteToOutputDir(projectDir)).To(BeNil())
	dockerYMLFile := path.Join(projectDir, BuildOutputDir, OutDockerFileName)
	_, err := os.Stat(dockerYMLFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+dockerYMLFile+" must exist")

	dockerEnvFile := path.Join(projectDir, BuildOutputDir, OutDockerEnvFileName)
	_, err = os.Stat(dockerEnvFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+dockerEnvFile+" must exist")

	dockerEnvFileBytes, _ := ioutil.ReadFile(dockerEnvFile)
	Expect(strings.TrimSpace(string(dockerEnvFileBytes))).To(Equal(
		"export MODULE_WITH_DASH_IMAGE_WITH_DASH_TAG=tag1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_IMAGE=image1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_DIGEST=digest1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_0_TAG=tag1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_0_IMAGE=image1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_0_DIGEST=digest1\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_1_TAG=tag2\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_1_IMAGE=image2\n" +
			"export MODULE_WITH_DASH_IMAGE_WITH_DASH_1_DIGEST=digest2"))
}

func ensureMavenEnvExists() (string, string, string) {
	homeDirPath, err := homedir.Dir()
	Expect(err).To(BeNil())
	mavenRepoPath := path.Join(homeDirPath, ".m2", "repository")
	err = os.MkdirAll(mavenRepoPath, os.ModePerm)
	if _, err := os.Stat(mavenRepoPath); os.IsNotExist(err) {
		Expect(errors.Wrapf(err, "could not create dir: %s", mavenRepoPath))
	}
	Expect(err).To(BeNil())
	settingsXmlPath := path.Join(homeDirPath, ".m2", "settings.xml")
	Expect(createFileIfNotExists(settingsXmlPath, `<settings></settings>`)).To(BeNil())
	settingsSecurityXmlPath := path.Join(homeDirPath, ".m2", "settings-security.xml")
	Expect(createFileIfNotExists(settingsSecurityXmlPath, `<settingsSecurity></settingsSecurity>`)).To(BeNil())
	return settingsXmlPath, settingsSecurityXmlPath, mavenRepoPath
}

func setupTempExampleProject(t *testing.T, pathToExample string) (*user.User, string, func()) {
	curUser, err := user.Current()
	require.NoError(t, err)
	dir, cleanup := createTempExampleProject(t, pathToExample)
	return curUser, dir, cleanup
}

func createFileIfNotExists(path string, content string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := ioutil.WriteFile(path, []byte(content), os.ModePerm); err != nil {
			return errors.Wrapf(err, "could not write file: %s", path)
		}
	}
	return nil
}
