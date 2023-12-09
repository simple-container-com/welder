package welder

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/util"
	. "github.com/simple-container-com/welder/pkg/welder/types"
)

func TestRunConditionalTasksOnlyWhenConditionIsTrue(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name            string
		runTask         string
		buildModule     string
		expectedContent string
		args            BuildArgs
		profiles        []string
	}{
		{
			name:            "run task expecting arg when arg specified",
			runTask:         "when-arg-specified",
			args:            BuildArgs{"expected-arg": "run"},
			expectedContent: "I am a task and running because arg is specified",
		},
		{
			name:            "run task not expecting arg when arg not specified",
			runTask:         "when-arg-not-specified",
			expectedContent: "I am a task and running because arg is not specified",
		},
		{
			name:    "run task expecting arg when arg not specified",
			runTask: "when-arg-specified",
		},
		{
			name:    "run task not expecting arg when arg specified",
			args:    BuildArgs{"expected-arg": "run"},
			runTask: "when-arg-not-specified",
		},
		{
			name: "run build when profile not active and arg not specified",
			expectedContent: "" +
				"I am a step and running because bamboo is not active\n" +
				"I am a step and running because arg is not specified\n" +
				"I am a task and running because bamboo is not active\n" +
				"I am a task and running because arg is not specified",
			buildModule: "some-conditional-module",
		},
		{
			name:     "run build when profile is active and arg not specified",
			profiles: []string{"bamboo"},
			expectedContent: "" +
				"I am a step and running because bamboo is active\n" +
				"I am a step and running because arg is not specified\n" +
				"I am a task and running because bamboo is active\n" +
				"I am a task and running because arg is not specified",
			buildModule: "some-conditional-module",
		},
		{
			name:     "run build when profile is active and arg is specified",
			profiles: []string{"bamboo"},
			args:     BuildArgs{"expected-arg": "run"},
			expectedContent: "" +
				"I am a step and running because bamboo is active\n" +
				"I am a step and running because arg is specified\n" +
				"I am a task and running because bamboo is active\n" +
				"I am a task and running because arg is specified",
			buildModule: "some-conditional-module",
		},
	}
	for _, testCase := range testCases {
		tc := testCase // for proper closures
		t.Run(tc.name, func(t *testing.T) {
			_, projectDir, cleanup := setupTempExampleProject(t, "testdata/conditional")
			defer cleanup()

			logger := util.NewPrefixLogger("[build]", false)
			buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{BuildArgs: tc.args, Profiles: tc.profiles}}, logger)
			buildCtx.SetRootDir(projectDir)

			if tc.runTask != "" {
				Expect(buildCtx.Run(tc.runTask, 0, tc.runTask)).To(BeNil())
			} else if tc.buildModule != "" {
				buildCtx.CommonCtx.Modules = []string{tc.buildModule}
				Expect(buildCtx.Build()).To(BeNil())
			}

			outputFile := path.Join(projectDir, "output")
			_, err := os.Stat(outputFile)
			if tc.expectedContent != "" {
				Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
				outputFileBytes, _ := ioutil.ReadFile(outputFile)
				Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal(tc.expectedContent))
			} else {
				Expect(os.IsNotExist(err)).To(Equal(true), "file "+outputFile+" must not exist")
			}
		})
	}
}

func TestTasksWithProjectPlaceholders(t *testing.T) {
	RegisterTestingT(t)

	_, projectDir, cleanup := setupTempExampleProject(t, "testdata/placeholders")
	defer cleanup()

	logger := util.NewPrefixLogger("[run]", false)
	buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{Profiles: []string{"bamboo"}, Strict: true}}, logger)
	buildCtx.SetRootDir(projectDir)

	Expect(buildCtx.Run("", 0, "some-task")).To(BeNil())
	outputFile := path.Join(projectDir, "output")
	_, err := os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal("" +
		"project:root=" + projectDir + "\n" +
		"project:version=1.0\n" +
		"profile:bamboo.active=true\n" +
		"project:module.name=${project:module.name}"))
}

func TestRunPlaceholders(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name            string
		profiles        []string
		ciName          string
		modeSox         bool
		modeVerbose     bool
		modeNoCache     bool
		modeOnHost      bool
		modeSkipTests   bool
		modeSyncMode    SyncMode
		expectedContent string
	}{
		{
			name:          "all modes and profiles enabled on bamboo",
			profiles:      []string{"custom"},
			ciName:        "bamboo",
			modeSkipTests: true,
			modeSox:       true,
			modeSyncMode:  SyncModeBind,
			expectedContent: "" +
				"mode-bamboo=true\n" +
				"mode-pipelines=false\n" +
				"mode-ci=true\n" +
				"mode-sox=true\n" +
				"mode-skip-tests=true\n" +
				"mode-verbose=false\n" +
				"mode-no-cache=false\n" +
				"mode-on-host=false\n" +
				"mode-sync-mode=bind\n" +
				"profile-bamboo=true\n" +
				"profile-pipelines=false\n" +
				"profile-sox=true\n" +
				"profile-skip-tests=true\n" +
				"profile-custom=true",
		},
		{
			name:          "all modes and profiles enabled on pipelines",
			profiles:      []string{"custom"},
			ciName:        "bitbucket-pipelines",
			modeSkipTests: true,
			modeSox:       true,
			modeSyncMode:  SyncModeBind,
			expectedContent: "" +
				"mode-bamboo=false\n" +
				"mode-pipelines=true\n" +
				"mode-ci=true\n" +
				"mode-sox=true\n" +
				"mode-skip-tests=true\n" +
				"mode-verbose=false\n" +
				"mode-no-cache=false\n" +
				"mode-on-host=false\n" +
				"mode-sync-mode=bind\n" +
				"profile-bamboo=false\n" +
				"profile-pipelines=true\n" +
				"profile-sox=true\n" +
				"profile-skip-tests=true\n" +
				"profile-custom=true",
		},
		{
			name:         "sox enabled",
			modeSox:      true,
			modeVerbose:  true,
			modeOnHost:   true,
			modeNoCache:  true,
			modeSyncMode: SyncModeAdd,
			ciName:       "some-unsupported-yet-ci-name",
			expectedContent: "" +
				"mode-bamboo=false\n" +
				"mode-pipelines=false\n" +
				"mode-ci=false\n" +
				"mode-sox=true\n" +
				"mode-skip-tests=false\n" +
				"mode-verbose=true\n" +
				"mode-no-cache=true\n" +
				"mode-on-host=true\n" +
				"mode-sync-mode=add\n" +
				"profile-bamboo=false\n" +
				"profile-pipelines=false\n" +
				"profile-sox=true\n" +
				"profile-skip-tests=false\n" +
				"profile-custom=false",
		},
	}
	for _, testCase := range testCases {
		tc := testCase // for proper closures
		t.Run(tc.name, func(t *testing.T) {
			_, projectDir, cleanup := setupTempExampleProject(t, "testdata/placeholders")
			defer cleanup()

			logger := util.NewPrefixLogger("[build]", false)
			buildCtx := NewBuildContext(&BuildContext{
				CommonCtx: &CommonCtx{
					Profiles:         tc.profiles,
					Verbose:          tc.modeVerbose,
					SoxEnabled:       tc.modeSox,
					SkipTestsEnabled: tc.modeSkipTests,
					NoCache:          tc.modeNoCache,
					SyncMode:         tc.modeSyncMode,
					ForceOnHost:      tc.modeOnHost,
				},
			}, logger)
			if tc.ciName != "" {
				buildCtx.CurrentCI = util.CurrentCI{Name: tc.ciName}
			}
			buildCtx.SetRootDir(projectDir)

			Expect(buildCtx.Build()).To(BeNil())

			outputFile := path.Join(projectDir, "output")
			_, err := os.Stat(outputFile)
			if tc.expectedContent != "" {
				Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
				outputFileBytes, _ := ioutil.ReadFile(outputFile)
				Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal(tc.expectedContent))
			} else {
				Expect(os.IsNotExist(err)).To(Equal(true), "file "+outputFile+" must not exist")
			}
		})
	}
}
