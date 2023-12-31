package pipelines

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/util"
)

func TestPopulatePipelinesEnvVariables(t *testing.T) {
	RegisterTestingT(t)

	pipelines, _, callback := preparePipelinesProject(t, "with-branch", "master")
	defer callback()

	pipelines.CurrentCI = util.CurrentCI{Name: "unsupported-ci"}

	env, err := pipelines.PipelinesEnv()
	Expect(err).To(BeNil())

	Expect(env).To(ContainElement("BITBUCKET_BUILD_NUMBER=0"))
	Expect(env).To(ContainElement(fmt.Sprintf("BITBUCKET_CLONE_DIR=%s", pipelines.RootDir())))
	Expect(env).To(ContainElement(fmt.Sprintf("BITBUCKET_BRANCH=master")))
}
