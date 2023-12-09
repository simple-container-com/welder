package pipelines

import (
	"fmt"
	"regexp"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/util"
	"gopkg.in/yaml.v2"
)

// BitbucketPipeVariable represents a Bitbucket pipe variable
type BitbucketPipeVariable struct {
	Name    string `yaml:"name"`
	Default string `yaml:"default"`
}

// BitbucketPipeDescriptor is a descriptor for a BitbucketPipe
type BitbucketPipeDescriptor struct {
	Name        string                  `json:"name"`
	Image       string                  `json:"image"`
	Variables   []BitbucketPipeVariable `json:"variables"`
	Description string                  `json:"description"`
	Repository  string                  `json:"repository"`
}

// BitbucketPipe is a pipe (part of the pipeline) that runs on Bitbucket CI.
type BitbucketPipe struct {
	*BitbucketContext
	Name      string
	Variables []string
}

func NewPipe(name string, ctx *BitbucketContext) *BitbucketPipe {
	return &BitbucketPipe{
		Name:             name,
		BitbucketContext: ctx,
	}
}

var pipeImageRegexp, _ = regexp.Compile(`(?P<prefix>docker:\/\/)?((?P<host>[\w,\-\_\.]+)(\/))?(?P<workspace>[\w,\-\_]+)\/(?P<repo>[\w,\-,\_]+):(?P<tag>[\w,\-,\_\.\d]+)?(@sha256:(?P<digest>[\w,\d]+))?`)

func (pipe *BitbucketPipe) DockerImage() (string, error) {
	match := util.MatchGroupsWithNames(pipeImageRegexp, pipe.Name)
	if match["prefix"] == "" && match["host"] == "" {
		// if no host and prefix are specified, we need to resolve pipe.yml from BB repository
		bbApiUrl := fmt.Sprintf("%s/repositories/%s/%s/src/%s/pipe.yml", MainBitbucketApiUrl, match["workspace"], match["repo"], match["tag"])
		pipeYml, err := pipe.FetchFromBitbucketAPI(bbApiUrl)
		if err != nil {
			return "", errors.Wrapf(err, "failed to fetch pipe.yml for pipe %s from %s", pipe.Name, bbApiUrl)
		}
		descriptor := BitbucketPipeDescriptor{}
		if err := yaml.Unmarshal([]byte(pipeYml), &descriptor); err != nil {
			return "", errors.Wrapf(err, "failed to unmarshal pipe.yml from  %s", bbApiUrl)
		}
		return descriptor.Image, nil
	}
	if match["host"] != "" && match["digest"] != "" {
		return fmt.Sprintf("%s/%s/%s:%s@sha256:%s", match["host"], match["workspace"], match["repo"], match["tag"], match["digest"]), nil
	} else if match["host"] != "" {
		return fmt.Sprintf("%s/%s/%s:%s", match["host"], match["workspace"], match["repo"], match["tag"]), nil
	} else if match["digest"] != "" {
		return fmt.Sprintf("%s/%s:%s@sha256:%s", match["workspace"], match["repo"], match["tag"], match["digest"]), nil
	}
	return fmt.Sprintf("%s/%s:%s", match["workspace"], match["repo"], match["tag"]), nil
}

func (ctx *BitbucketContext) parseBitbucketRemote() (BitbucketRemote, error) {
	res := BitbucketRemote{}

	remotes, err := ctx.GitClient().Remotes()
	if err != nil {
		return res, errors.Wrapf(err, "failed to get git remotes")
	}
	for _, remote := range remotes {
		for _, url := range remote.URLs {
			if gitRepoUrlRegexp.Match([]byte(url)) {
				match := util.MatchGroupsWithNames(gitRepoUrlRegexp, url)
				res.Workspace = match["workspace"]
				res.RepoSlug = match["repo"]
				res.Name = remote.Name
				res.FullName = fmt.Sprintf("%s/%s", match["workspace"], match["repo"])
				break
			}
		}
	}
	return res, nil
}

func (pipe *BitbucketPipe) Run(runCtx *docker.RunContext) error {
	pipe.Logger().Logf("+ %s", pipe.Name)

	image, err := pipe.DockerImage()
	if err != nil {
		return errors.Wrapf(err, "failed to get docker image for pipe %s", pipe.Name)
	}
	runID := docker.CleanupDockerID(fmt.Sprintf("%s-%s", pipe.projectName, pipe.Name))
	dockerRun, err := pipe.NewDockerRun(runID, pipe.Name, image)
	if err != nil {
		return errors.Wrapf(err, "failed to create docker run for pipe %s", pipe.Name)
	}
	newRunCtx := runCtx.Clone()
	newRunCtx.Env = append(newRunCtx.Env, pipe.Variables...)
	return dockerRun.
		SetDisableCache(pipe.NoCache).
		SetEnv(newRunCtx.Env...).
		SetContext(pipe.GoContext()).
		Run(newRunCtx)
}
