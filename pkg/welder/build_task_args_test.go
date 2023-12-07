package welder

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	. "github.com/smecsia/welder/pkg/welder/types"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestOverrideArgsInTasksWorksAsExpected(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name            string
		runTask         string
		expectedContent string
		args            BuildArgs
		module          string
		profiles        []string
	}{
		{
			name:            "run task-no-arg expecting default uses default's arg",
			runTask:         "task-no-arg",
			expectedContent: "from-default",
		},
		{
			name:            "run task-no-arg when profile enabled prefers profile's arg",
			runTask:         "task-no-arg",
			profiles:        []string{"test-profile"},
			expectedContent: "from-profile",
		},
		{
			name:            "run task-no-arg when arg specified uses provided arg",
			runTask:         "task-no-arg",
			args:            BuildArgs{"test-arg": "from-arg"},
			expectedContent: "from-arg",
		},
		{
			name:            "run task-with-arg when profile specified prefers task's arg",
			runTask:         "task-with-arg",
			profiles:        []string{"test-profile"},
			expectedContent: "from-task",
		},
		{
			name:            "run task-with-arg when arg not specified prefers task's arg",
			runTask:         "task-with-arg",
			expectedContent: "from-task",
		},
		{
			name:            "run task-with-arg when arg specified uses provided arg",
			runTask:         "task-with-arg",
			args:            BuildArgs{"test-arg": "from-arg"},
			expectedContent: "from-arg",
		},
		{
			name:            "run task-with-arg with module-with-arg prefers task's arg",
			runTask:         "task-with-arg",
			module:          "module-with-arg",
			expectedContent: "from-task",
		},
		{
			name:            "run task-no-arg with module-with-arg prefers module's arg",
			runTask:         "task-no-arg",
			module:          "module-with-arg",
			expectedContent: "from-module",
		},
	}
	for _, testCase := range testCases {
		tc := testCase // for proper closures
		t.Run(tc.name, func(t *testing.T) {
			_, projectDir, cleanup := setupTempExampleProject(t, "testdata/task-args-override")
			defer cleanup()

			logger := util.NewPrefixLogger("[build]", false)
			buildCtx := NewBuildContext(&BuildContext{CommonCtx: &CommonCtx{BuildArgs: tc.args, Profiles: tc.profiles}}, logger)
			if tc.module != "" {
				buildCtx.Modules = []string{tc.module}
			}
			buildCtx.SetRootDir(projectDir)

			Expect(buildCtx.Run(tc.runTask, 0, tc.runTask)).To(BeNil())

			outputFile := path.Join(projectDir, "output")
			_, err := os.Stat(outputFile)
			Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
			outputFileBytes, _ := ioutil.ReadFile(outputFile)
			Expect(strings.TrimSpace(string(outputFileBytes))).To(Equal(tc.expectedContent))
		})
	}
}
