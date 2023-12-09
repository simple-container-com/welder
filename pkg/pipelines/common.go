package pipelines

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/pipelines/schema"
	"github.com/simple-container-com/welder/pkg/welder/runner"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

const (
	MainBitbucketApiUrl        = "https://api.bitbucket.org/2.0"
	AlternativeBitbucketApiUrl = "https://bitbucket.org/!api/2.0"
)

type BitbucketContext struct {
	*types.CommonCtx
	BuildNumber int
	OAuthToken  string
	JWTToken    string
	SlauthToken string
	Branch      string

	config      schema.BitbucketPipelinesSchemaJson
	filePath    string
	projectRoot string
	projectName string
}

type BitbucketRepoResponse struct {
	Uuid      string `json:"uuid"`
	Workspace struct {
		Uuid string `json:"uuid"`
	} `json:"workspace"`
}

func NewBitbucketContext(ctx *types.CommonCtx) *BitbucketContext {
	return &BitbucketContext{
		CommonCtx: ctx,
	}
}

func (ctx *BitbucketContext) WithOAuthToken(token string) *BitbucketContext {
	ctx.OAuthToken = token
	return ctx
}

func (ctx *BitbucketContext) WithSlauthToken(token string) *BitbucketContext {
	ctx.SlauthToken = token
	return ctx
}

func (ctx *BitbucketContext) WithConfig(config schema.BitbucketPipelinesSchemaJson) *BitbucketContext {
	ctx.config = config
	return ctx
}

func (ctx *BitbucketContext) WithConfigFile(filePath string) *BitbucketContext {
	ctx.filePath = filePath
	return ctx
}

func (ctx *BitbucketContext) WithProjectName(projectName string) *BitbucketContext {
	ctx.projectName = projectName
	return ctx
}

func (ctx *BitbucketContext) WithProjectRoot(rootDir string) *BitbucketContext {
	ctx.projectRoot = rootDir
	return ctx
}

func (ctx *BitbucketContext) WithBuildNumber(number int) *BitbucketContext {
	ctx.BuildNumber = number
	return ctx
}

// BitbucketRemote represents a Bitbucket remote
type BitbucketRemote struct {
	Workspace string
	RepoSlug  string
	FullName  string
	Name      string
}

var gitRepoUrlRegexp = regexp.MustCompile(`(ssh:\/\/|git@|https:\/\/)(?P<host>[\w\.@]+)(\/|:)(?P<workspace>[\w,\-,\_]+)\/(?P<repo>[\w,\-,\_]+)(.git){0,1}((\/){0,1})`)

func (ctx *BitbucketContext) PipelinesEnv() ([]string, error) {
	// if already running within Bitbucket Pipelines, just proxy all specific Bitbucket environment variable values
	if ctx.IsRunningInBitbucketPipelines() {
		return (&types.BuildEnv{}).ToBuildEnv(regexp.MustCompile("BITBUCKET_.*"), regexp.MustCompile("PIPELINES_.*")), nil
	}

	res := []string{
		"CI=true",
		fmt.Sprintf("BITBUCKET_BUILD_NUMBER=%d", ctx.BuildNumber),
		fmt.Sprintf("BITBUCKET_CLONE_DIR=%s", ctx.RootDir()),
	}

	if ctx.Branch != "" {
		res = append(res, fmt.Sprintf("BITBUCKET_BRANCH=%s", ctx.Branch))
	} else if branch, err := ctx.GitClient().Branch(); err != nil {
		ctx.Logger().Errf("failed to get current branch: %s", err.Error())
	} else {
		res = append(res, fmt.Sprintf("BITBUCKET_BRANCH=%s", branch))
	}

	if commit, err := ctx.GitClient().Hash(); err != nil {
		ctx.Logger().Errf("failed to get current commit: %s", err.Error())
	} else {
		res = append(res, fmt.Sprintf("BITBUCKET_COMMIT=%s", commit))
	}

	if remote, err := ctx.parseBitbucketRemote(); err != nil {
		ctx.Logger().Errf("failed to parse Bitbucket remote: %s", err.Error())
	} else {
		res = append(res, []string{
			fmt.Sprintf("BITBUCKET_WORKSPACE=%s", remote.Workspace),
			fmt.Sprintf("BITBUCKET_GIT_HTTP_ORIGIN=http://bitbucket.org/%s", remote.FullName),
			fmt.Sprintf("BITBUCKET_GIT_SSH_ORIGIN=git@bitbucket.org:/%s.git", remote.FullName),
			fmt.Sprintf("BITBUCKET_REPO_SLUG=%s", remote.RepoSlug),
			fmt.Sprintf("BITBUCKET_REPO_FULL_NAME=%s", remote.FullName),
		}...)

		if resp, err := ctx.FetchFromBitbucketAPI(fmt.Sprintf("%s/repositories/%s/%s", AlternativeBitbucketApiUrl, remote.Workspace, remote.RepoSlug)); err != nil {
			ctx.Logger().Errf("failed to resolve Bitbucket repository remote: %s", err.Error())
		} else {
			repoResponse := BitbucketRepoResponse{}
			if err := json.Unmarshal([]byte(resp), &repoResponse); err != nil {
				ctx.Logger().Errf("failed to unmarshal Bitbucket repository response: %s", err.Error())
			}
			res = append(res, []string{
				fmt.Sprintf("BITBUCKET_REPO_UUID=%s", repoResponse.Uuid),
				fmt.Sprintf("BITBUCKET_REPO_OWNER_UUID=%s", repoResponse.Workspace.Uuid),
			}...)
		}
	}

	if ctx.JWTToken != "" {
		res = append(res, fmt.Sprintf("PIPELINES_JWT_TOKEN=%s", ctx.JWTToken))
	}

	return res, nil
}

func (ctx *BitbucketContext) FetchFromBitbucketAPI(bbApiUrl string) (string, error) {
	req, err := http.NewRequest("GET", bbApiUrl, nil)
	if err != nil {
		return "", errors.Wrapf(err, "failed to make request to %s", bbApiUrl)
	}
	if ctx.OAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ctx.OAuthToken))
	}
	resp, err := http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		return "", errors.Errorf("failed to fetch response from %s: %s", bbApiUrl, resp.Status)
	}
	if err != nil {
		return "", errors.Wrapf(err, "failed to fetch response from %s", bbApiUrl)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read response from %s", bbApiUrl)
	}
	return string(body), nil
}

func (ctx *BitbucketContext) NewDockerRun(runID string, name string, image string) (*docker.Run, error) {
	run := runner.NewRun(ctx.CommonCtx)

	env, err := ctx.PipelinesEnv()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain pipelines env variables")
	}

	runCfg := types.CommonRunDefinition{
		CommonSimpleRunDefinition: types.CommonSimpleRunDefinition{
			Env:     types.ParseBuildEnv(env),
			Volumes: nil,
			WorkDir: ctx.projectRoot,
		},
	}

	params, err := run.CalcRunInContainerParams(ctx.projectName, ctx.projectRoot, runCfg, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to calc run params")
	}
	spec := types.RunSpec{
		Name:   name,
		Image:  image,
		RunCfg: runCfg,
	}
	dockerRun, err := docker.NewRun(runID, spec.Image)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init docker runner with image %s", spec.Image)
	}

	if err := run.ConfigureVolumes(dockerRun, params); err != nil {
		return nil, errors.Wrapf(err, "failed to configure volumes")
	}

	return dockerRun, nil
}
