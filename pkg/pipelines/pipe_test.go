package pipelines

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/util"
	"os"
	"runtime"
	"testing"
)

func TestResolveDockerImageFromPipe(t *testing.T) {
	RegisterTestingT(t)

	if (&util.CurrentCI{}).IsRunningInBitbucketPipelines() || os.Getenv("INSIDE_WELDER") == "true" {
		t.Skip("skipping test because it should only be run manually")
	}

	testCases := []struct {
		pipeName      string
		expectedImage string
	}{
		{
			pipeName:      "atlassian/aws-cloudformation-deploy:0.5.0",
			expectedImage: "bitbucketpipelines/aws-cloudformation-deploy:0.5.0",
		},
		{
			pipeName:      "atlassian/git-secrets-scan:0.6.1",
			expectedImage: "bitbucketpipelines/git-secrets-scan:0.6.1",
		},
		{
			pipeName:      "smecsia/bitbucket-empty-pipe:v1",
			expectedImage: "library/hello-world:latest",
		},
	}
	for _, testCase := range testCases {
		tc := testCase // for proper closures
		t.Run(tc.pipeName, func(t *testing.T) {

			pipe := NewPipe(tc.pipeName, &BitbucketContext{})
			//token, err := GenerateBitbucketOauthToken(false)
			//Expect(err).To(BeNil())
			//pipe.OAuthToken = token
			image, err := pipe.DockerImage()
			Expect(err).To(BeNil())
			Expect(image, err).To(Equal(tc.expectedImage))
		})
	}
}

func TestRunPipe(t *testing.T) {
	RegisterTestingT(t)

	if (&util.CurrentCI{}).IsRunningInBitbucketPipelines() || os.Getenv("INSIDE_WELDER") == "true" || runtime.GOARCH == "arm64" {
		t.Skip("skipping test because it should only be run manually")
	}
	pipelines, _, callback := preparePipelinesProject(t, "with-branch", "master")
	defer callback()

	pipe := NewPipe("atlassian/artifactory-sidekick:v1", pipelines.BitbucketContext)
	//token, err := GenerateBitbucketOauthToken(false)
	//Expect(err).To(BeNil())
	//pipe.OAuthToken = token
	env, err := pipe.PipelinesEnv()
	Expect(err).To(BeNil())
	Expect(pipe.Run(&docker.RunContext{
		WorkDir: pipe.RootDir(),
		Env:     env,
	})).To(BeNil())

}
