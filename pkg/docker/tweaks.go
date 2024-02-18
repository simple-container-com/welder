package docker

import (
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/git"
	"github.com/simple-container-com/welder/pkg/socksy"
	"github.com/simple-container-com/welder/pkg/util"
)

const (
	LabelNameSSHAuthPort         = "WelderSSHAuthPort"
	BitbucketPipelinesSshKeyPath = "/opt/atlassian/pipelines/agent/ssh/id_rsa"
)

type tweakAction func(dockerAPI *client.Client, containerID string)

type containerCreateTweak func(config *container.Config, hostConfig *container.HostConfig, netConfig *network.NetworkingConfig)

// extraTweak represents some extra tweaking required for better integration with host system
type extraTweak struct {
	extraBinds            []string
	extraVolumes          []Volume
	extraInitCommands     []ExecContext
	extraBuildCommands    []string
	extraActions          []tweakAction
	containerCreateTweaks []containerCreateTweak
	extraEnv              []string
	extraContainerLabels  map[string]string
	proxiesToHost         []proxyToHost
}

type addrType string

const (
	proxyTypeUnix = addrType("unix")
	proxyTypeTcp  = addrType("tcp")
)

type proxyToHost struct {
	source string
	target string
}

func newTweak() *extraTweak {
	return &extraTweak{
		extraVolumes:          make([]Volume, 0),
		extraInitCommands:     make([]ExecContext, 0),
		extraBuildCommands:    make([]string, 0),
		extraActions:          make([]tweakAction, 0),
		extraEnv:              make([]string, 0),
		containerCreateTweaks: make([]containerCreateTweak, 0),
		extraBinds:            make([]string, 0),
		extraContainerLabels:  make(map[string]string),
		proxiesToHost:         make([]proxyToHost, 0),
	}
}

func (t *extraTweak) addContainerCreateTweaks(tweak ...containerCreateTweak) {
	t.containerCreateTweaks = append(t.containerCreateTweaks, tweak...)
}

func (t *extraTweak) addInitCmds(cmd ...ExecContext) {
	t.extraInitCommands = append(t.extraInitCommands, cmd...)
}

func (t *extraTweak) addProxyToHost(proxy proxyToHost) {
	t.proxiesToHost = append(t.proxiesToHost, proxy)
}

func (t *extraTweak) addContainerLabel(name, value string) {
	t.extraContainerLabels[name] = value
}

func (t *extraTweak) addBuildCmds(cmd ...string) {
	t.extraBuildCommands = append(t.extraBuildCommands, cmd...)
}

func (t *extraTweak) addBinds(v ...string) {
	t.extraBinds = append(t.extraBinds, v...)
}

func (t *extraTweak) addVolumes(v ...Volume) {
	t.extraVolumes = append(t.extraVolumes, v...)
}

func (t *extraTweak) addActions(a ...tweakAction) {
	t.extraActions = append(t.extraActions, a...)
}

func (t *extraTweak) addEnv(env ...string) {
	t.extraEnv = append(t.extraEnv, env...)
}

// extraIntegrationsExistingContainer runs some actions on existing container (if reuse is enabled)
func (run *Run) extraIntegrationsExistingContainer(runCtx RunContext, containerID string) error {
	runningContainer, err := run.dockerAPI.ContainerInspect(run.GoContext(), containerID)
	if err != nil {
		return errors.Wrapf(err, "failed to inspect container %s", containerID)
	}

	// re-integrate SSH hacks if container had them
	if val, ok := runningContainer.Config.Labels[LabelNameSSHAuthPort]; ok {
		sshAuthSock := os.Getenv("SSH_AUTH_SOCK")
		proxy := socksy.NewUnixSocketProxy(sshAuthSock)
		port, err := strconv.Atoi(val)
		if err != nil {
			return errors.Wrapf(err, "failed to convert port value to int: %s", val)
		}
		_, err = proxy.Start(port, runCtx.logger(true))
		if err != nil {
			return errors.Wrapf(err, "failed to start SSH proxy")
		}
	}
	return nil
}

// extraSystemIntegrations returns a number of tweaks required for better integration with host system
// and list of extra volumes
// and list of init commands needed to be executed prior the main commands are started
// and list of build commands needed to be executed when building Docker image
// and list of extra actions needed to be executed upon Docker container
// this method should be safe to call and must never fail
// it must explicitly ignore errors and should log every error into the debug output
func (run *Run) extraSystemIntegrations(runCtx RunContext) *extraTweak {
	res := newTweak()

	// add extra network
	res.addActions(func(dockerAPI *client.Client, containerID string) {
		net, err := run.createDockerNetwork(runCtx, containerID)
		if err != nil {
			runCtx.Debugf("failed to create Docker network: %s ", err.Error())
		}
		run.network = net
	})

	// add extra git integration volumes
	res.addVolumes(run.getExtraGitMounts(runCtx)...)

	// add extra commands only when base image contains supported Linux distribution
	if run.osDistribution.IsLinuxBased() {
		// add make sure user exists build commands
		res.addBuildCmds(run.getMakeSureUserExistsCommands(runCtx)...)
		res.addBuildCmds(run.getPrecreateVolumedDirs(runCtx)...)
		userChownCommands := run.getEnsureHomeDirectoryPermissionsCommands(runCtx)
		res.addBuildCmds(userChownCommands...)

		// additionally make sure user owns home dir
		res.addActions(func(dockerAPI *client.Client, containerID string) {
			rootCtx := runCtx.CloneAs("root", "/")
			for _, chownCmd := range userChownCommands {
				if _, err := run.execSingleCommand(containerID, svcCommand(rootCtx, chownCmd)); err != nil {
					runCtx.Debugf("failed to change owner of home directory: %s ", err.Error())
				}
			}
		})

		// make sure sleep exists in the container
		// res.addBuildCmds("sleep 0.1 || rm -f /bin/sleep && echo '#!/bin/sh \\\\ \n while sleep $1; do :; done' > /bin/sleep && chmod +x /bin/sleep && ls -la /bin/sleep")

		// add SSH tweaks to the result
		run.addSSHTweaks(runCtx, res)
	}

	// add Docker-in-Docker tweaks
	run.addUseDockerInDockerNetworkTweaks(runCtx, res)

	// Add Mac-OS specific Volume user / group changes
	run.macOsVolumePermissionsFix(runCtx, res)

	return res
}

// macOsVolumePermissionsFix ensures any volumes added have permissions to work with the "simulated" user in container (defaults to root)
func (run *Run) macOsVolumePermissionsFix(runCtx RunContext, tweak *extraTweak) {
	if runCtx.isMacOS() {
		rootCtx := runCtx.CloneAs("root", "/")
		for _, v := range run.volumeBinds {
			volume := v
			tweak.addActions(func(dockerAPI *client.Client, containerID string) {
				chmodCmd := svcCommand(rootCtx, fmt.Sprintf("chown -R %s %s:%s %s %s || true", runCtx.VerboseFlag("-v"), runCtx.User, runCtx.User, volume.ContPath, runCtx.cmdSuffix()))
				if _, err := run.execSingleCommand(containerID, chmodCmd); err != nil {
					runCtx.Warnf("failed to change owner of volume path %s: %s", volume.ContPath, err.Error())
				}
				chgrpCmd := svcCommand(rootCtx, fmt.Sprintf("chgrp -R %s %s", runCtx.User, volume.ContPath))
				if _, err := run.execSingleCommand(containerID, chgrpCmd); err != nil {
					runCtx.Warnf("WARN: failed to change group permissions of volume path %s: %s", volume.ContPath, err.Error())
				}
			})
		}
	}
}

// addUseDockerInDockerNetworkTweaks adds docker in docker tweaks if necessary
func (run *Run) addUseDockerInDockerNetworkTweaks(runCtx RunContext, tweak *extraTweak) {
	if run.Util().IsRunningInDocker() {
		runCtx.Debugf("Activating tweaks for running in-Docker environment")
		tweak.addContainerCreateTweaks(func(config *container.Config, hostConfig *container.HostConfig, netConfig *network.NetworkingConfig) {
			runCtx.Debugf("Looking for own networks")
			networks, err := run.Util().FindSelfDockerNetworks()
			if err != nil {
				runCtx.Warnf("failed to find own networks: %s", err.Error())
				return
			}
			if len(networks) > 0 {
				runCtx.Debugf("First own network is: %s", networks[0].Name)
				// adding first container network as the main network for vault e2e environment (so Docker host would be accessible)
				netConfig.EndpointsConfig = make(map[string]*network.EndpointSettings)
				netConfig.EndpointsConfig[networks[0].Name] = &network.EndpointSettings{}
			} else {
				runCtx.Warnf("No own Docker networks found, although it seems we're running in Docker")
			}
		})

	}

	if run.mountDockerSocket {
		if run.Util().IsDockerHostRemote() { // add integration with Docker daemon running locally or remotely
			dockerHost := run.Util().DockerHost()
			runCtx.Debugf("DOCKER_HOST specified: %s, creating tcp proxy within container...", dockerHost)
			if dockerURL, err := url.Parse(dockerHost); err != nil {
				runCtx.Warnf("Failed to parse Docker host %s: %s", dockerHost, err.Error())
			} else {
				proxy := socksy.NewTcpProxy(fmt.Sprintf(dockerURL.Host))
				port, err := proxy.Start(0, runCtx.logger(true))
				if err != nil {
					runCtx.Warnf("failed to start Docker host proxy (%s): %s", dockerHost, err.Error())
				} else {
					runCtx.Debugf("started proxy for Docker host socket (%s) at 0.0.0.0:%d", dockerHost, port)
					ipOfNetwork := runCtx.HostnameOfHost()
					if ips, err := socksy.GetExternalIPs(); err != nil {
						runCtx.Warnf("Failed to list all available external IPs of host: %s", err.Error())
					} else if len(ips) > 1 {
						runCtx.Warnf(fmt.Sprintf("Could not determine single network interface. Selecting the first one: %s", ips[0]))
						ipOfNetwork = ips[0].IP
					} else if len(ips) == 1 {
						ipOfNetwork = ips[0].IP
					} else {
						runCtx.Warnf("No external IPs for host found!")
					}
					internalDockerHost := fmt.Sprintf("DOCKER_HOST=tcp://%s:%d", ipOfNetwork, port)
					runCtx.Debugf("injecting DOCKER_HOST=%s", internalDockerHost)
					tweak.addEnv(internalDockerHost)
				}
			}
		} else { // add docker.sock volume (default)
			runCtx.Debugf("adding bind mount for Docker: %s", DockerSockPath)
			tweak.addBinds(fmt.Sprintf("%s:%s", DockerSockPath, DockerSockPath))
			rootCtx := runCtx.CloneAs("root", "/")
			tweak.addActions(func(dockerAPI *client.Client, containerID string) {
				// Docker in Docker environment needs some preparations to function properly
				chmodCmd := svcCommand(rootCtx, fmt.Sprintf("chmod %s +rwx %s", runCtx.VerboseFlag("-v"), DockerSockPath))
				if _, err := run.execSingleCommand(containerID, chmodCmd); err != nil {
					runCtx.Warnf("failed to change permissions of docker.sock: %s", err.Error())
				}
				// hack to make it work under linux with a system defined user and keep same permissions
				if runCtx.User != "root" && runCtx.User != "" {
					chgrpCmd := svcCommand(rootCtx, fmt.Sprintf("chgrp %s %s", runCtx.User, DockerSockPath))
					if _, err := run.execSingleCommand(containerID, chgrpCmd); err != nil {
						runCtx.Warnf("WARN: failed to change permissions of docker.sock: %s", err.Error())
					}
				}
			})
		}

		tweak.addActions(func(dockerAPI *client.Client, containerID string) {
			// copy ~/.docker directory to follow same credentials as the host environment
			if err := run.copyDockerCredentialsToContainer(runCtx, containerID); err != nil {
				runCtx.Warnf("failed to copy Docker creds to container: %s", err.Error())
			}
		})
	}
}

func (run *Run) copyDockerCredentialsToContainer(runCtx RunContext, containerID string) error {
	homeCfg, err := ReadDockerConfigJson()
	if err != nil {
		return errors.Wrapf(err, "failed to read config.json")
	}

	err = homeCfg.ResolveExternalAuths()
	if err != nil {
		return errors.Wrapf(err, "failed to resolve external registry auth")
	}

	// no need to generate config.json if it's empty
	if homeCfg.IsEmpty() {
		return nil
	}

	cfgJsonFile, err := homeCfg.DumpToTmpFile()
	defer os.RemoveAll(cfgJsonFile)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve external registry auth")
	}

	homeDockerDir := fmt.Sprintf("/home/%s/.docker", runCtx.User)
	if runCtx.User == "root" || runCtx.User == "" {
		homeDockerDir = "/root/.docker"
	}
	if _, err := run.execSingleCommand(containerID, svcCommand(runCtx, run.preCreateDirectoryCmd(runCtx, homeDockerDir))); err != nil {
		return err
	}

	return run.copyToContainer(runCtx, containerID, cfgJsonFile, fmt.Sprintf("%s/config.json", homeDockerDir))
}

// addSSHTweaks adds different SSH tweaking into the tweak object
func (run *Run) addSSHTweaks(runCtx RunContext, tweak *extraTweak) {
	// NB: the following integration tries to proxy ssh agent from host to container via
	// retargeting ~/.ssh directory and then moving all files from /tmp/home_ssh to ~/.ssh
	sshDir := runCtx.homeDir() + "/.atlas_build_home_ssh"
	sshMountFound, needsOverride := run.processSshDirMount(runCtx, sshDir)
	if sshMountFound { // this is needed only if ~/.ssh directory is requested as one of the mounts

		// in Bitbucket Pipelines we need to add extra volume bind if it wasn't added already
		if runCtx.CurrentCI.IsRunningInBitbucketPipelines() {
			found := false
			for _, bind := range run.volumeBinds {
				if bind.HostPath == BitbucketPipelinesSshKeyPath {
					found = true
				}
			}
			if !found {
				if _, err := os.Stat(BitbucketPipelinesSshKeyPath); err == nil {
					tweak.addVolumes(Volume{
						HostPath: BitbucketPipelinesSshKeyPath,
						ContPath: BitbucketPipelinesSshKeyPath,
						Mode:     VolumeModeRO,
					})
				}
			}
		}

		// Appending ssh-agent related things into the container
		// For Linux we can only forward the socket and SSH_AUTH_SOCK environment variable
		// For MacOS refer: https://github.com/docker/for-mac/issues/410 and https://github.com/docker/for-mac/issues/483
		sshAuthSock := os.Getenv("SSH_AUTH_SOCK")

		// TODO: use bundled socat instead of installing it via package manager
		if sshAuthSock != "" && runCtx.isLinux() {
			runCtx.Debugf("host system is Linux, can integrate SSH via volume")
			tweak.addVolumes(Volume{HostPath: sshAuthSock, ContPath: sshAuthSock, Mode: VolumeModeRW})
			tweak.addEnv(fmt.Sprintf("SSH_AUTH_SOCK=%s", sshAuthSock))
			tweak.addBuildCmds(run.osDistribution.InstallPackageCommands("openssh-client", runCtx.cmdSuffix()))
		} else if sshAuthSock != "" && runCtx.isMacOS() {
			// MacOS ssh command is different and supports some options unavailable in Linux
			runCtx.Debugf("host system is MacOS, need to use workarounds for SSH integrations...")
			// We're trying to use this workaround here: https://gist.github.com/cowlicks/1c0d6973b894d46b1a0ea6cd99bc7852
			proxy := socksy.NewUnixSocketProxy(sshAuthSock)
			port, err := proxy.Start(0, runCtx.logger(true))
			if err != nil {
				runCtx.Debugf("failed to start proxy to ssh agent socket (%s): %s", sshAuthSock, err.Error())
			} else {
				runCtx.Debugf("started proxy for ssh agent socket (%s) at 0.0.0.0:%d", sshAuthSock, port)
				tweak.addBuildCmds(run.osDistribution.InstallPackageCommands("socat", runCtx.cmdSuffix()))
				tweak.addBuildCmds(run.osDistribution.InstallPackageCommands("openssh-client", runCtx.cmdSuffix()))
				tweak.addEnv("SSH_AUTH_SOCK=/tmp/auth.sock")
				tweak.addContainerLabel(LabelNameSSHAuthPort, strconv.Itoa(port))
				tweak.addInitCmds(svcCommandDetached(runCtx.CloneAs("root", "/"),
					fmt.Sprintf(`socat UNIX-LISTEN:${SSH_AUTH_SOCK},unlink-early,mode=777,fork TCP:%s:%d`, HostSystemHostname, port),
				))
			}
		}
		if needsOverride {
			tweak.addInitCmds(svcCommandDetached(runCtx,
				fmt.Sprintf("rm -fR ~/.ssh && mkdir -p ~/.ssh && cp -fR %s/* ~/.ssh && ssh-add -l || true", sshDir),
			))
		}

		if runCtx.isMacOS() {
			// MacOS ssh command is different and supports some options unavailable in Linux
			runCtx.Debugf("host system is MacOS, need to use workarounds for SSH integrations...")
			if cmd, err := run.integrateSSHConfigProperly(runCtx); err != nil {
				runCtx.Debugf("failed to integrate ssh_config: %s", err.Error())
			} else {
				tweak.addInitCmds(cmd)
			}
		}
	}
}

// processSshDirMount checks whether ~/.ssh directory integration has been requested
// returns whether if volume containing /.ssh is found and if it needs to be overriden
// this method is safe to call and returns -1 in case of error. It logs errors into the debug output
// NB: this method may not work if ssh directory is different from default one
func (run *Run) processSshDirMount(runCtx RunContext, overrideContPath string) (bool, bool) {
	homeSshDir, err := homedir.Expand("~/.ssh")
	if err != nil {
		runCtx.Debugf("failed to expand on ~/.ssh directory: %s", err.Error())
		return false, false
	}
	// find if there's ~/.ssh directory among the requested volumes
	for idx, v := range run.volumeBinds {
		if strings.HasPrefix(v.HostPath, homeSshDir) {
			run.volumeBinds[idx].ContPath = overrideContPath
			run.volumeBinds[idx].Mode = VolumeModeRO // ~/.ssh must be always read-only
			return true, true
		}
	}
	// find if there's /.ssh directory among the mounts
	for _, v := range run.volumeMounts {
		if strings.Contains(v.ContPath, "/.ssh") {
			return true, false
		}
	}
	return false, false
}

// integrateSSHConfigProperly integrates ssh config properly
func (run *Run) integrateSSHConfigProperly(runCtx RunContext) (ExecContext, error) {
	homeSshDir, err := homedir.Expand("~/.ssh")
	if err != nil {
		return ExecContext{}, errors.Wrapf(err, "failed to expand on ~/.ssh directory")
	}
	// integrating ~/.ssh/config file
	sshConfigFile := filepath.Join(homeSshDir, "config")
	_, err = os.Stat(filepath.Join(sshConfigFile))
	if err != nil && !os.IsNotExist(err) {
		return ExecContext{}, errors.Wrapf(err, "failed to read ssh config from %s", sshConfigFile)
	}
	// UseKeychain is a new option available on MacOS, which isn't available on Linux hence we need to remove it
	// from ssh configuration. See more at https://developer.apple.com/library/archive/technotes/tn2449/_index.html
	return svcCommandDetached(runCtx, "set -e; cat ~/.ssh/config | grep -v UseKeychain > /tmp/ssh_config; mv /tmp/ssh_config ~/.ssh/config"), nil
}

// getExtraGitMounts returns extra volumes necessary for Git to work properly into container. returns empty slice if any error occurs
func (run *Run) getExtraGitMounts(runCtx RunContext) []Volume {
	res := make([]Volume, 0)

	gitRoot, err := git.TraverseToRoot()
	if err != nil {
		runCtx.Debugf("failed to detect Git root: %s", err.Error())
		return res
	}

	if alternates, err := gitRoot.Alternates(); err != nil {
		runCtx.Debugf("failed to read Git alternates: %s", err.Error())
	} else {
		for _, alternate := range alternates {
			res = append(res, Volume{HostPath: alternate, ContPath: alternate, Mode: VolumeModeRW})
		}
	}

	if worktrees, err := gitRoot.Worktrees(); err != nil {
		runCtx.Debugf("failed to read Git worktrees: %s", err.Error())
	} else {
		for _, wt := range worktrees {
			res = append(res, Volume{HostPath: wt, ContPath: wt, Mode: VolumeModeRW})
		}
	}

	return res
}

// getMakeSureUserExistsCommands returns list of commands that create user identical to the host user
func (run *Run) getMakeSureUserExistsCommands(runCtx RunContext) []string {
	res := make([]string, 0)
	if runCtx.User == "" || runCtx.User == "root" || !ValidUsernameRegex.Match([]byte(runCtx.User)) {
		return res
	}
	curUser, err := user.Current()
	if err != nil {
		runCtx.Debugf("failed to determine current user: %s", err.Error())
		return res
	}

	if curUser.Username != "root" { // this is necessary to allow container's user having the same user id and username as host's
		// removing user with the same ID or username as host user if it exists in the container
		res = append(res,
			fmt.Sprintf("DELUID=$(cat /etc/passwd | grep \":%s:\" | awk -F: \"{print \\$1}\"); deluser $DELUID %s || true; "+
				"deluser %s %s || true", curUser.Uid, runCtx.cmdSuffix(), curUser.Username, runCtx.cmdSuffix()))
		res = append(res,
			fmt.Sprintf("id -u %s 2>/dev/null || "+
				"adduser -D -u %s %s %s || "+
				"adduser -u %s %s %s || "+
				"adduser --disabled-password -u %s --gecos \"\" %s %s || true",
				runCtx.User,
				curUser.Uid, runCtx.User, runCtx.cmdSuffix(),
				curUser.Uid, runCtx.User, runCtx.cmdSuffix(),
				curUser.Uid, runCtx.User, runCtx.cmdSuffix()))
	} else {
		// add user with the requested username
		res = append(res,
			fmt.Sprintf("id -u %s 2>/dev/null || adduser -D %s %s || "+
				"adduser --disabled-password --gecos \"\" %s %s || true",
				runCtx.User, runCtx.User, runCtx.cmdSuffix(), runCtx.User, runCtx.cmdSuffix()))
	}

	if run.mountDockerSocket {
		// add user to Docker group respecting Alpine's & Debian's command arguments:
		res = append(res,
			fmt.Sprintf("groupadd docker %s || addgroup docker %s || true ; "+
				"usermod -a -G docker %s %s || adduser %s docker %s || true",
				runCtx.cmdSuffix(), runCtx.cmdSuffix(), runCtx.User, runCtx.cmdSuffix(), runCtx.User, runCtx.cmdSuffix()))
	}
	return res
}

func (run *Run) getEnsureHomeDirectoryPermissionsCommands(runCtx RunContext) []string {
	if runCtx.User == "" || runCtx.User == "root" || !ValidUsernameRegex.Match([]byte(runCtx.User)) {
		return nil
	}
	homeDir := fmt.Sprintf("/home/%s", runCtx.User)
	chownCmd := fmt.Sprintf("chown -R %s %s:%s %s %s || true", runCtx.VerboseFlag("-v"),
		runCtx.User, runCtx.User, homeDir, runCtx.cmdSuffix())
	return []string{chownCmd}
}

func (run *Run) getPrecreateVolumedDirs(runCtx RunContext) []string {
	res := make([]string, 0)
	for _, v := range run.volumeBinds {
		if cmd := run.precreateVolumeDirCmd(runCtx, v); cmd != "" {
			res = append(res, cmd)
		}
	}
	for _, v := range run.volumeMounts {
		if cmd := run.precreateVolumeDirCmd(runCtx, v); cmd != "" {
			res = append(res, cmd)
		}
	}
	return res
}

func (run *Run) precreateVolumeDirCmd(runCtx RunContext, v Volume) string {
	hostPath := run.resolveVolumeHostPath(runCtx, v)
	srcInfo, err := os.Stat(hostPath)
	var cmd string
	if err != nil {
		runCtx.Debugf("WARN: Failed to get stat of host volume %s -> %s: %s", v.HostPath, hostPath, err.Error())
	} else if srcInfo.IsDir() {
		cmd = run.preCreateDirectoryCmd(runCtx, v.ContPath)
	}
	return cmd
}

// createDockerNetwork creates Docker network and connects container to it
func (run *Run) createDockerNetwork(runCtx RunContext, containerID string) (NetworkData, error) {
	res := NetworkData{}
	// creating Network
	ctx := run.GoContext()
	netResp, err := run.dockerAPI.NetworkCreate(ctx, run.RunID, types.NetworkCreate{
		Driver: "bridge", Attachable: true, Labels: map[string]string{
			LabelNameContainerID: run.RunID,
			LabelNameConfigHash:  run.initialConfigHash,
		},
	})
	if err != nil {
		return res, errors.Wrapf(err, "failed to create network")
	}
	networkID := netResp.ID
	err = run.dockerAPI.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{})
	if err != nil {
		return res, errors.Wrapf(err, "failed to connect container %s to network %s", containerID, networkID)
	}

	netInfo, err := run.dockerAPI.NetworkInspect(ctx, networkID, types.NetworkInspectOptions{Verbose: true})
	if err != nil {
		return res, errors.Wrapf(err, "failed to inspect network %s", networkID)
	}

	if len(netInfo.IPAM.Config) == 0 {
		return res, errors.Errorf("created network doesn't contain IPAM info: %s", networkID)
	}

	res.Gateway = netInfo.IPAM.Config[0].Gateway
	res.ID = networkID

	// adding access to gateway via gateway's IP address via simple name "gateway"
	// In Linux it can be used to access Host environment
	// In MacOS we should use host.docker.internal instead (see https://docs.docker.com/docker-for-mac/networking/)
	if _, err := run.execSingleCommand(containerID, svcCommand(runCtx.CloneAs("root", "/"),
		fmt.Sprintf("echo '%s %s' >> /etc/hosts", res.Gateway, GatewayHostname))); err != nil {
		runCtx.Debugf("failed to add /etc/hosts entry: %s", err.Error())
	}

	return res, nil
}

func (run *Run) preCreateDirectoryCmd(runCtx RunContext, homeDockerDir string) string {
	cmd := fmt.Sprintf("mkdir -p %s", homeDockerDir)
	if runCtx.User != "" && runCtx.User != "root" && ValidUsernameRegex.Match([]byte(runCtx.User)) {
		cmd += fmt.Sprintf(" && chown -R %s %s:%s %s ",
			runCtx.VerboseFlag("-v"), runCtx.User, runCtx.User, homeDockerDir)
	}
	return cmd
}

func (ctx *RunContext) homeDir() string {
	if ctx.User == "root" || ctx.User == "" {
		return "/root"
	}
	return fmt.Sprintf("/home/%s", ctx.User)
}

// flags that assume bash is running in interactive mode
var interactiveBashFlags = []string{
	"-l", "-i",
}

// cleanupContainerCommand returns command that makes sure it can be executed in background
// example: "bash -l" or "bash -i" cannot be executed in background without proper TTY
func (ctx *RunContext) cleanupContainerCommand(cmd []string) []string {
	if len(cmd) > 1 && strings.HasSuffix(cmd[0], "bash") && util.SliceContains(interactiveBashFlags, cmd[1]) {
		return cmd[:1] // trim off the interactive flag
	}
	if len(cmd) == 1 && util.SliceContains(interactiveBashFlags, cmd[0]) {
		return []string{} // empty command
	}
	return cmd
}
