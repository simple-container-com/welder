package welder

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/lithammer/shortuuid/v3"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/welder/pkg/git/mock"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/runner"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestActiveProfiles(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")

	Expect(err).To(BeNil())

	activeProfiles := (&BuildContext{
		CommonCtx: &CommonCtx{SoxEnabled: true, Profiles: []string{"skip-tests"}},
	}).
		ActiveProfiles(&rootDef, "")

	Expect(activeProfiles).To(ContainElements("skip-tests", "sox"))

	activeProfiles = (&BuildContext{
		CommonCtx: &CommonCtx{SkipTestsEnabled: true},
	}).ActiveProfiles(&rootDef, "")

	Expect(activeProfiles).To(ContainElements("skip-tests"))

	activeProfiles = (&BuildContext{
		CommonCtx: &CommonCtx{CurrentCI: util.CurrentCI{"bamboo"}},
	}).ActiveProfiles(&rootDef, "")

	Expect(activeProfiles).To(ContainElements("bamboo"))

	activeProfiles = (&BuildContext{
		CommonCtx: &CommonCtx{SimulateOS: "linux"},
	}).ActiveProfiles(&rootDef, "")

	Expect(activeProfiles).To(ContainElements("linux"))

	oldValue := os.Getenv("bamboo_JWT_TOKEN")
	_ = os.Setenv("bamboo_JWT_TOKEN", "somevalue")
	defer os.Setenv("bamboo_JWT_TOKEN", oldValue)
	activeProfiles = (&BuildContext{CommonCtx: &CommonCtx{}}).ActiveProfiles(&rootDef, "")

	Expect(activeProfiles).To(ContainElements("bamboo"))
}

func TestActualDeployDefinitionFor(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")

	Expect(err).To(BeNil())

	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f", nil)
	gitMock.On("Branch").Return("feature/test", nil)
	buildCtx := &BuildContext{CommonCtx: &CommonCtx{SoxEnabled: true, CurrentCI: util.CurrentCI{Name: "bamboo"}}}
	buildCtx.SetGitClient(&gitMock)

	valueOfJson := "[{\"SOME_JSON\":\"SOME_VALUE\"}]"
	os.Setenv("bamboo_JWT_TOKEN", valueOfJson)
	defer os.Unsetenv("bamboo_JWT_TOKEN")

	thirdDeploy, module, err := buildCtx.ActualDeployDefinitionFor(&rootDef, "third", &DeployContext{Envs: []string{"dev"}})

	Expect(err).To(BeNil())
	Expect(module.Name).To(Equal("third"))

	Expect(thirdDeploy.Args).To(HaveKeyWithStringValue("flags", "--strict"))
	Expect(thirdDeploy.Env).To(HaveKeyWithStringValue("bamboo_JWT_TOKEN", valueOfJson))
	Expect(thirdDeploy.Env).To(HaveKeyWithStringValue("SOME_DEPLOY_VAR", "value"))
	Expect(thirdDeploy.Env).To(HaveKeyWithStringValue("SOME_OTHER_VAR", "value"))
	expectedRunDef := (&CommonRunDefinition{Args: BuildArgs{"some-env-var": "dev"}}).Init()
	Expect(thirdDeploy.Environments).To(HaveKeyWithValue("ddev",
		DeployEnvDefinition{AutoDeploy: true, CommonRunDefinition: *expectedRunDef}))
	expectedRunDef = (&CommonRunDefinition{
		Args:                      BuildArgs{"some-env-var": "staging"},
		CommonSimpleRunDefinition: CommonSimpleRunDefinition{Env: BuildEnv{"SOME_DEPLOY_VAR": "staging-value"}},
	}).Init()
	Expect(thirdDeploy.Environments).To(HaveKeyWithValue("stg-west",
		DeployEnvDefinition{AutoDeploy: true, CommonRunDefinition: *expectedRunDef}))
	Expect(thirdDeploy.Steps[0].Step.Scripts[0]).To(Equal("mkdir -p services/armory/target"))
	Expect(thirdDeploy.Steps[0].Step.Scripts[1]).To(Equal("echo '1.0.0-beta' > services/armory/target/deploy-version"))
	Expect(thirdDeploy.Steps[0].Step.Scripts[2]).To(Equal("echo '--strict dev' > services/armory/target/deploy-flags"))
}

func TestActualBuildDefinition(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")

	Expect(err).To(BeNil())

	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("fullfullhash", nil)
	gitMock.On("Branch").Return("branchname", nil)

	buildCtx := &BuildContext{
		CommonCtx: &CommonCtx{
			SoxEnabled: true, Profiles: []string{"skip-tests"},
			BuildArgs: BuildArgs{"extra": "lalala"}, Username: "bob",
		},
	}
	buildCtx.SetGitClient(&gitMock)
	armoryBuildDef, module, err := buildCtx.ActualBuildDefinitionFor(&rootDef, "armory")

	Expect(err).To(BeNil())
	Expect(module.Name).To(Equal("armory"))

	Expect(armoryBuildDef.Env).To(Equal(BuildEnv{
		"ENV_WITH_DEFAULT_ARG":      "defaultEnvValue",
		"ENV_FROM_ARG_WITH_DEFAULT": "defaultArgValue",
		"BUILD_ARGS":                "-Dsome.other.var=blah",
		"SOME_OTHER_VAR":            "value",
		"EXTRA_VAR":                 "lalala",
		"MAVEN_OPTS":                "-Xms512M -Xmx1024M -Xss2M -XX:MaxMetaspaceSize=1024M",
	}))

	Expect(armoryBuildDef.Args).To(Equal(BuildArgs{
		"arg-with-default": "defaultArgValue",
		"extra":            "lalala",
		"namespace":        "docker.simple-container.com/test/deng/sox",
		"maven-version":    "3.8.6-openjdk-18-slim",
		"project-version":  "0.0.2-fullfullhash",
	}))

	Expect(armoryBuildDef.Volumes).To(ConsistOf(
		"~/.m2/settings.xml:/home/bob/.m2/settings.xml:ro",
		"~/.m2/settings-security.xml:/home/bob/.m2/settings-security.xml:ro"))
	Expect(armoryBuildDef.Steps[0].Step.Image).To(Equal("maven:3.8.6-openjdk-18-slim"))
	Expect(armoryBuildDef.Steps[0].Step.Scripts[4]).To(Equal("echo ${BUILD_ARGS}"))

	buildCtx = &BuildContext{
		CommonCtx: &CommonCtx{
			Profiles:  []string{},
			BuildArgs: BuildArgs{"extra": "hohoho", "maven-version": "1.0"},
		},
	}
	buildCtx.SetGitClient(&gitMock)
	trebuchetBuildDef, module, err := buildCtx.ActualBuildDefinitionFor(&rootDef, "trebuchet")

	Expect(err).To(BeNil())
	Expect(module.Name).To(Equal("trebuchet"))

	Expect(trebuchetBuildDef.Env).To(Equal(BuildEnv{
		"ENV_WITH_DEFAULT_ARG":      "defaultEnvValue",
		"ENV_FROM_ARG_WITH_DEFAULT": "defaultArgValue",
		"BUILD_ARGS":                "",
		"SOME_OTHER_VAR":            "value",
		"MAVEN_OPTS":                "-Xms512M -Xmx1024M -Xss2M -XX:MaxMetaspaceSize=1024M",
		"PROJECT_VERSION":           "0.0.1-fullful-trebuchet",
	}))

	Expect(trebuchetBuildDef.Args).To(Equal(BuildArgs{
		"arg-with-default": "defaultArgValue",
		"extra":            "hohoho",
		"namespace":        "docker.simple-container.com/test/deng",
		"maven-version":    "1.0",
		"project-version":  "0.0.1-fullful-trebuchet",
	}))

	Expect(trebuchetBuildDef.Steps[0].Step.Image).To(Equal("maven:1.0"))
	Expect(trebuchetBuildDef.Steps[0].Step.Scripts[4]).To(Equal("echo \"\""))
}

func TestActualTaskDefinitionForModule(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")

	Expect(err).To(BeNil())

	buildCtx := &BuildContext{
		CommonCtx: &CommonCtx{
			Profiles:  []string{"sox"},
			BuildArgs: BuildArgs{"extra": "lalala"},
		},
	}
	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f-test", nil)
	gitMock.On("Branch").Return("branch-test", nil)
	buildCtx.SetGitClient(&gitMock)

	deployCtx := &DeployContext{
		BuildContext: buildCtx,
		Envs:         []string{"ddev"},
	}
	echoTaskDefinition, err := buildCtx.ActualTaskDefinitionFor(&rootDef, "do-echo", "trebuchet", deployCtx)

	Expect(err).To(BeNil())

	Expect(echoTaskDefinition.Env).To(Equal(BuildEnv{
		"ENV_WITH_DEFAULT_ARG":      "defaultEnvValue",
		"ENV_FROM_ARG_WITH_DEFAULT": "defaultArgValue",
		"BUILD_ARGS":                "-Dsome.other.var=blah",
		"SOME_OTHER_VAR":            "value",
		"EXTRA_VAR":                 "lalala",
		"PROJECT_VERSION":           "0.0.1-1234567-trebuchet",
		"MAVEN_OPTS":                "-Xms512M -Xmx1024M -Xss2M -XX:MaxMetaspaceSize=1024M",
		"SOME_DEPLOY_VAR":           "value",
	}))

	Expect(echoTaskDefinition.Args).To(Equal(BuildArgs{
		"arg-with-default": "defaultArgValue",
		"extra":            "lalala",
		"namespace":        "docker.simple-container.com/test/deng/sox",
		"maven-version":    "3.8.6-openjdk-18-slim",
		"project-version":  "0.0.1-1234567-trebuchet",
		"some-env-var":     "dev",
		"flags":            "--strict",
	}))

	Expect(echoTaskDefinition.Volumes).To(ConsistOf(
		"~/.m2/settings.xml:/root/.m2/settings.xml:ro",
		"~/.m2/settings-security.xml:/root/.m2/settings-security.xml:ro"))
	Expect(echoTaskDefinition.Image).To(Equal("maven:3.8.6-openjdk-18-slim"))
	Expect(echoTaskDefinition.ContainerWorkDir).To(Equal("/some/directory"))
	Expect(echoTaskDefinition.Scripts[0]).To(Equal("echo \"${BUILD_ARGS} ${EXTRA_VAR}\""))

	echoTaskDefinition, err = buildCtx.ActualTaskDefinitionFor(&rootDef, "print-home-dir", "trebuchet", deployCtx)
	Expect(err).To(BeNil())
	Expect(echoTaskDefinition.ContainerWorkDir).To(Equal("services"))

	echoTaskDefinition, err = buildCtx.ActualTaskDefinitionFor(&rootDef, "print-home-dir", "fourth", deployCtx)
	Expect(err).To(BeNil())
	Expect(echoTaskDefinition.ContainerWorkDir).To(Equal("/some/default/directory"))
}

func TestActualTaskDefinition(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")

	Expect(err).To(BeNil())

	buildCtx := &BuildContext{
		CommonCtx: &CommonCtx{
			Profiles:  []string{"sox"},
			BuildArgs: BuildArgs{"extra": "lalala"},
		},
	}
	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f", nil)
	gitMock.On("Branch").Return("feature/test", nil)
	buildCtx.SetGitClient(&gitMock)
	echoTaskDefinition, err := buildCtx.ActualTaskDefinitionFor(&rootDef, "do-echo", "", nil)

	Expect(err).To(BeNil())

	Expect(echoTaskDefinition.Env).To(Equal(BuildEnv{
		"ENV_WITH_DEFAULT_ARG":      "defaultEnvValue",
		"ENV_FROM_ARG_WITH_DEFAULT": "defaultArgValue",
		"BUILD_ARGS":                "-Dsome.other.var=blah",
		"SOME_OTHER_VAR":            "value",
		"EXTRA_VAR":                 "lalala",
		"MAVEN_OPTS":                "-Xms512M -Xmx1024M -Xss2M -XX:MaxMetaspaceSize=1024M",
	}))

	Expect(echoTaskDefinition.Args).To(Equal(BuildArgs{
		"arg-with-default": "defaultArgValue",
		"extra":            "lalala",
		"namespace":        "docker.simple-container.com/test/deng/sox",
		"maven-version":    "3.8.6-openjdk-18-slim",
		"project-version":  "0.0.1-1234567-${project:module.name}",
	}))

	Expect(echoTaskDefinition.Volumes).To(ConsistOf(
		"~/.m2/settings.xml:/root/.m2/settings.xml:ro",
		"~/.m2/settings-security.xml:/root/.m2/settings-security.xml:ro"))
	Expect(echoTaskDefinition.Image).To(Equal("maven:3.8.6-openjdk-18-slim"))
	Expect(echoTaskDefinition.Scripts[0]).To(Equal("echo \"${BUILD_ARGS} ${EXTRA_VAR}\""))
}

func TestActualDockerDefinition(t *testing.T) {
	RegisterTestingT(t)
	rootDef, err := ReadBuildRootDefinition("testdata/example")
	Expect(err).To(BeNil())

	gitMock := mock.GitMock{}
	gitMock.On("Hash").Return("1234567890f", nil)
	gitMock.On("Branch").Return("feature/test", nil)
	buildCtx := &BuildContext{
		CommonCtx: &CommonCtx{
			Profiles:  []string{"skip-tests"},
			BuildArgs: BuildArgs{"maven-version": "1.0"},
		},
	}
	buildCtx.SetGitClient(&gitMock)
	armoryDockerImages, err := buildCtx.ActualDockerImagesDefinitionFor(&rootDef, "armory")
	Expect(err).To(BeNil())
	Expect(armoryDockerImages).To(HaveLen(1))
	Expect(armoryDockerImages[0].Tags).To(HaveLen(2))
	Expect(armoryDockerImages[0].Tags).To(ConsistOf("docker.simple-container.com/test/deng/armory:1234567", "docker.simple-container.com/test/deng/armory:1234567890f"))
	Expect(armoryDockerImages[0].Build.Args[0]).To(Equal(DockerBuildArg{Name: "module", Value: "armory"}))
	Expect(armoryDockerImages[0].Build.Args[1]).To(Equal(DockerBuildArg{Name: "maven-version", Value: "1.0"}))

	buildCtx = &BuildContext{CommonCtx: &CommonCtx{SoxEnabled: true}}
	buildCtx.SetGitClient(&gitMock)
	trebuchetDockerImages, err := buildCtx.ActualDockerImagesDefinitionFor(&rootDef, "trebuchet")
	Expect(err).To(BeNil())
	Expect(trebuchetDockerImages).To(HaveLen(1))
	Expect(trebuchetDockerImages[0].Tags).To(HaveLen(2))
	Expect(trebuchetDockerImages[0].Tags).To(ConsistOf("docker.simple-container.com/test/deng/sox/trebuchet:1234567", "docker.simple-container.com/test/deng/sox/trebuchet:1234567890f"))
	Expect(trebuchetDockerImages[0].Build.Args[0]).To(Equal(DockerBuildArg{Name: "module", Value: "trebuchet"}))
	Expect(trebuchetDockerImages[0].Build.Args[1]).To(Equal(DockerBuildArg{Name: "maven-version", Value: "3.8.6-openjdk-18-slim"}))

	thirdDockerImages, err := buildCtx.ActualDockerImagesDefinitionFor(&rootDef, "third")
	Expect(err).To(BeNil())
	Expect(thirdDockerImages).To(HaveLen(1))
	Expect(thirdDockerImages[0].DockerFile).To(Equal("./Dockerfile"))
	Expect(thirdDockerImages[0].Tags).To(HaveLen(1))
	Expect(thirdDockerImages[0].Tags).To(ConsistOf("docker.simple-container.com/test/deng/sox/third:1234567"))
	Expect(thirdDockerImages[0].Build.Args[0]).To(Equal(DockerBuildArg{Name: "module", Value: "third"}))
}

func createTempExampleProject(t *testing.T, pathToExample string) (string, func()) {
	depDir := path.Join(runner.WelderTempDir(), fmt.Sprintf("example-%s", shortuuid.New()[:5]))
	err := copy.Copy(pathToExample, depDir)
	require.NoError(t, err)
	return depDir, func() {
		_ = os.RemoveAll(depDir)
	}
}

func HaveKeyWithStringValue(key interface{}, value string) types.GomegaMatcher {
	return HaveKeyWithValue(key, StringValue(value))
}
