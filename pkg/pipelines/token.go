package pipelines

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/exec"
	"github.com/simple-container-com/welder/pkg/util"
)

var atlasPluginsPath = []string{".local", "share", "atlassian", "atlas", "plugin"}

type AtlasCli struct {
	Executor exec.Exec `json:"-"`
}

// TODO: fix after open sourcing
func GenerateBitbucketOauthToken(proxyExec bool) (string, error) {
	atlasCli := AtlasCli{Executor: exec.NewExec(context.Background(), &util.NoopLogger{})}
	if err := atlasCli.EnsureAtlasPluginInstalled("slauth"); err != nil {
		return "", errors.Wrapf(err, "failed to install slauth plugin")
	}
	output, err := atlasCli.RunAtlasCommand("slauth oauth --audience bitbucket.org --scopes account --output http", proxyExec)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate bitbucket oauth token: %s", output)
	}
	parts := strings.Split(strings.TrimRight(output, "\n "), " ")
	if len(parts) < 2 {
		return "", errors.Errorf("failed to obtain bitbucket oauth token from response: %q", output)
	}
	return parts[len(parts)-1], nil
}

func (o *AtlasCli) EnsureAtlasPluginInstalled(pluginName string) error {
	if exists, err := CheckHomeFileExists(append(atlasPluginsPath, pluginName)...); err != nil {
		return err
	} else if !exists {
		return o.InstallAtlasPlugin(pluginName)
	}
	return nil
}

func (o *AtlasCli) RunAtlasCommand(command string, proxyExec bool) (string, error) {
	return o.RunCommand(fmt.Sprintf("atlas %s", command), proxyExec)
}

func (o *AtlasCli) RunCommand(command string, proxyExec bool) (string, error) {
	return o.RunCommandWithOpts(command, proxyExec, exec.Opts{})
}

func (o *AtlasCli) RunCommandWithOpts(command string, proxyExec bool, opts exec.Opts) (string, error) {
	if proxyExec {
		return "", o.Executor.ProxyExec(command, opts)
	}
	return o.Executor.ExecCommand(command, opts)
}

func (o *AtlasCli) InstallAtlasPlugin(name string) error {
	if err := o.Executor.ProxyExec(fmt.Sprintf("atlas plugin install -n %s", name), exec.Opts{}); err != nil {
		return err
	}
	return nil
}

func CheckHomeFileExists(homePath ...string) (bool, error) {
	usr, err := user.Current()
	if err != nil {
		return false, errors.Wrapf(err, "failed to get current user")
	}
	fullHomePath := path.Join(append([]string{usr.HomeDir}, homePath...)...)
	if _, err := os.Stat(fullHomePath); err != nil && os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrapf(err, "failed to check file exists: %s", homePath)
	}
	return true, nil
}
