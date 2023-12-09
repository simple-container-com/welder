package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/lithammer/shortuuid/v3"
	"github.com/pkg/errors"
)

const (
	dockerIpV4NotationRegex = `(\d+\.\d+\.\d+\.\d+)(/\d+)?`
)

// DockerUtil useful Docker utils
type DockerUtil struct {
	docker        client.APIClient
	cgroupContent string
	context       context.Context
}

// ContainerStatus returns status of container
type ContainerStatus struct {
	Exists   bool
	Running  bool
	ExitCode int
}

// NewDefaultUtil returns default util
func NewDefaultUtil(context context.Context) (*DockerUtil, error) {
	dockerAPI, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	dockerAPI.NegotiateAPIVersion(context)
	dockerUtil, err := NewDockerUtil(dockerAPI, context)
	return dockerUtil, nil
}

// NewDockerUtil initialize new Docker utils object
func NewDockerUtil(dockerClient client.APIClient, context context.Context) (*DockerUtil, error) {
	if dockerClient == nil {
		docker, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			return nil, err
		}
		docker.NegotiateAPIVersion(context)
		dockerClient = docker
	}
	res := DockerUtil{docker: dockerClient, context: context}
	if cgroupContentBytes, err := ioutil.ReadFile("/proc/1/cgroup"); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "failed to read cgroups file")
	} else if err != nil {
		res.cgroupContent = ""
	} else {
		res.cgroupContent = string(cgroupContentBytes)
	}
	return &res, nil
}

// GoContext returns execution context if set
func (u *DockerUtil) GoContext() context.Context {
	if u.context == nil {
		return context.Background()
	}
	return u.context
}

// DisconnectFromContainerNetworks disconnects current container from other container's network
func (u *DockerUtil) DisconnectFromContainerNetworks(containerIDOrName string) error {
	currentContainerID, err := u.currentContainerID()
	if err != nil {
		return errors.Wrapf(err, "failed to determine current container's ID")
	}
	otherContainerNetworks, err := u.FindDockerNetworksOf(containerIDOrName)
	if err != nil {
		return errors.Wrapf(err, "failed to detect other container's network")
	}
	// networks found, let's disconnect
	for _, network := range otherContainerNetworks {
		if err := u.docker.NetworkDisconnect(u.GoContext(), network.ID, currentContainerID, true); err != nil {
			return errors.Wrapf(err, "failed to disconnect from Docker network")
		}
	}
	return nil
}

// CleanupContainerNetworks disconnects all containers from all networks of the provided container
func (u *DockerUtil) CleanupContainerNetworks(containerIDOrName string) error {
	otherContainerNetworks, err := u.FindDockerNetworksOf(containerIDOrName)
	if err != nil {
		return errors.Wrapf(err, "failed to detect other container's network")
	}
	ctx := u.GoContext()
	for _, network := range otherContainerNetworks {
		// disconnect all existing containers from network
		networkInfo, _ := u.docker.NetworkInspect(ctx, network.ID, types.NetworkInspectOptions{Verbose: true})
		for _, c := range networkInfo.Containers {
			_ = u.docker.NetworkDisconnect(ctx, network.ID, c.Name, true)
		}
		if err := u.docker.NetworkRemove(ctx, network.ID); err != nil {
			return errors.Wrapf(err, "failed to remove network %q for container %q", network.Name, containerIDOrName)
		}
	}
	return nil
}

// ConnectToContainerNetworks connects current container to other container's network
func (u *DockerUtil) ConnectToContainerNetworks(containerIDOrName string) (string, error) {
	currentContainerID, err := u.currentContainerID()
	if err != nil {
		return "", errors.Wrapf(err, "failed to determine current container's ID")
	}
	otherContainerNetworks, err := u.FindDockerNetworksOf(containerIDOrName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to detect other container's network")
	}
	for _, otherContainerNetwork := range otherContainerNetworks {
		// network found, let's connect to it
		fmt.Println("Connecting container ", currentContainerID, " to network ", otherContainerNetwork.Name, "...")
		if err := u.docker.NetworkConnect(u.GoContext(), otherContainerNetwork.ID, currentContainerID, &networktypes.EndpointSettings{
			Links: []string{currentContainerID}, Aliases: []string{currentContainerID},
		}); err != nil {
			return "", errors.Wrapf(err, "failed to connect to Docker network")
		}
		for _, container := range otherContainerNetwork.Containers {
			if container.Name == containerIDOrName || container.EndpointID == containerIDOrName {
				// now we should be able to access it
				return dockerIPv4ToAddrString(container.IPv4Address), nil
			}
		}
	}
	return "", nil
}

func (u *DockerUtil) FindContainerIPv4Addr(containerIDOrName string) (string, error) {
	otherContainerNetworks, err := u.FindDockerNetworksOf(containerIDOrName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to detect container networks")
	}
	for _, otherContainerNetwork := range otherContainerNetworks {
		for _, container := range otherContainerNetwork.Containers {
			if container.Name == containerIDOrName || container.EndpointID == containerIDOrName {
				return dockerIPv4ToAddrString(container.IPv4Address), nil
			}
		}
	}
	return "", nil
}

func (u *DockerUtil) FindSelfDockerNetworks() ([]types.NetworkResource, error) {
	selfContainerID, err := u.currentContainerID()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect own container id")
	}
	return u.FindDockerNetworksOf(selfContainerID)
}

func (u *DockerUtil) FindDockerNetworksOf(nameOrID string) ([]types.NetworkResource, error) {
	res := make([]types.NetworkResource, 0)
	container, err := u.docker.ContainerInspect(u.GoContext(), nameOrID)
	if err != nil {
		return res, errors.Wrapf(err, "failed to inspect container %s", nameOrID)
	}
	// use own container IP address directly if running in Docker-In-Docker environment
	if netList, err := u.docker.NetworkList(u.GoContext(), types.NetworkListOptions{}); err != nil {
		return res, errors.Wrapf(err, "failed to list Docker networks")
	} else {
		for _, net := range netList {
			network, _ := u.docker.NetworkInspect(u.GoContext(), net.Name, types.NetworkInspectOptions{})
			for _, containerEP := range network.Containers {
				if "/"+containerEP.Name == container.Name || containerEP.EndpointID == container.ID {
					res = append(res, network)
				}
			}
		}
		if len(res) == 0 {
			return res, errors.Errorf("no networks found for container %q", container.ID)
		}
	}
	return res, nil
}

func dockerIPv4ToAddrString(ipv4 string) string {
	return regexp.MustCompile(dockerIpV4NotationRegex).ReplaceAllString(ipv4, "$1")
}

func (u *DockerUtil) currentContainerID() (string, error) {
	reader := bufio.NewReader(bytes.NewReader([]byte(u.cgroupContent)))
	var err error
	var nextLine string
	for err == nil {
		nextLine, err = reader.ReadString('\n')
		if strings.Contains(nextLine, "docker/") || strings.Contains(nextLine, "kubepods/") {
			parts := strings.Split(nextLine, ":")
			resParts := strings.Split(parts[2], "/")
			return resParts[len(resParts)-1][:12], nil
		}
	}
	return "", fmt.Errorf("failed to detect current container's ID. cgroups contents:\n %s", u.cgroupContent)
}

// ReadFileFromContainer reads file from container
func (u *DockerUtil) ReadFileFromContainer(containerName string, filePath string) (string, error) {
	tmpFile, err := ioutil.TempFile("", containerName)
	if err != nil {
		return "", err
	}

	defer func(name string) {
		_ = os.Remove(name)
	}(tmpFile.Name())

	stat, err := u.docker.ContainerStatPath(u.GoContext(), containerName, filePath)
	if err != nil {
		return "", err
	}

	// resolve symlinks
	if stat.LinkTarget != "" {
		filePath = stat.LinkTarget
	}

	content, _, err := u.docker.CopyFromContainer(u.GoContext(), containerName, filePath)
	if err != nil {
		return "", err
	}

	defer func(content io.ReadCloser) {
		_ = content.Close()
	}(content)

	srcInfo := archive.CopyInfo{
		Path:   filePath,
		Exists: true,
	}

	err = archive.CopyTo(content, srcInfo, tmpFile.Name())

	if err != nil {
		return "", err
	}

	contBytes, err := ioutil.ReadFile(tmpFile.Name())

	return string(contBytes), err
}

// ForceRemoveContainer kills and removes container
func (u *DockerUtil) ForceRemoveContainer(containerID string, timeout time.Duration) error {
	// ignore possible stop issues
	ctx := u.GoContext()
	_ = u.docker.ContainerStop(ctx, containerID, &timeout)
	_ = u.docker.ContainerKill(ctx, containerID, "KILL")
	return u.docker.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
}

// GetContainerStatus returns true if container exists
func (u *DockerUtil) GetContainerStatus(containerID string) (ContainerStatus, error) {
	res := ContainerStatus{}
	inspResp, err := u.docker.ContainerInspect(u.GoContext(), containerID)
	if err != nil && client.IsErrNotFound(err) {
		return res, nil
	} else if err != nil {
		return res, err
	}
	res.Exists = true
	res.Running = inspResp.State.Running
	res.ExitCode = inspResp.State.ExitCode
	return res, nil
}

// InspectDockerImage returns inspect of the Docker Image
func (u *DockerUtil) InspectDockerImage(reference string) (types.ImageInspect, error) {
	inspResp, _, err := u.docker.ImageInspectWithRaw(u.GoContext(), reference)
	if err != nil {
		return types.ImageInspect{}, errors.Wrapf(err, "failed to inspect Docker image %s", reference)
	}
	return inspResp, nil
}

// WaitUntilContainerExits waits until container exits
func (u *DockerUtil) WaitUntilContainerExits(containerID string) error {
	chok, cherr := u.docker.ContainerWait(u.GoContext(), containerID, container.WaitConditionNotRunning)
	for {
		select {
		case <-chok:
			return nil
		case err := <-cherr:
			return err
		}
	}
}

// StreamContainerLogsTo returns true if container exists
func (u *DockerUtil) StreamContainerLogsTo(containerID string, stdout io.Writer, stderr io.Writer) error {
	reader, err := u.docker.ContainerLogs(u.GoContext(), containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = stdcopy.StdCopy(stdout, stderr, reader)
	return err
}

// ExecInContainer executes command in container and returns output as a string
func (u *DockerUtil) ExecInContainer(containerID string, command string) (string, error) {
	execConfig := types.ExecConfig{
		Privileged:   false,
		Cmd:          []string{"/bin/sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  false,
		Detach:       false,
		Tty:          false,
	}

	ctx := u.GoContext()
	crResp, err := u.docker.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", err
	}

	hjResp, err := u.docker.ContainerExecAttach(ctx, crResp.ID, types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	})
	if err != nil {
		return "", err
	}

	respBytes, err := ioutil.ReadAll(hjResp.Reader)
	if err != nil {
		return "", err
	}

	inspResp, err := u.docker.ContainerExecInspect(ctx, crResp.ID)
	if err != nil {
		return "", err
	}
	if inspResp.ExitCode != 0 {
		return string(respBytes), errors.Errorf("non-zero exit code for command %q (running: %t)", command, inspResp.Running)
	}

	return string(respBytes), nil
}

func (u *DockerUtil) VolumeExists(name string) bool {
	_, err := u.docker.VolumeInspect(u.GoContext(), name)
	if err != nil {
		return false
	}
	return true
}

func (u *DockerUtil) CreateVolume(name string, labels map[string]string) error {
	if _, err := u.docker.VolumeCreate(u.GoContext(), volume.VolumeCreateBody{
		Driver: "local",
		Name:   name,
	}); err != nil {
		return err
	}
	return nil
}

func (u *DockerUtil) CopyToContainer(hostPath string, containerID string, dstDirPath string) error {
	tar, err := archive.TarWithOptions(hostPath, &archive.TarOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to create tar archive out of the path: %s", hostPath)
	}

	err = u.docker.CopyToContainer(u.GoContext(), containerID, dstDirPath, tar, types.CopyToContainerOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to copy files into container %s from path: %s", containerID, hostPath)
	}
	return nil
}

func (u *DockerUtil) VolumeRemove(name string) error {
	return u.docker.VolumeRemove(u.GoContext(), name, true)
}

// IsRunningInDocker returns true if currently running inside Docker container
func (u *DockerUtil) IsRunningInDocker() bool {
	// fmt.Println("check if running in Docker...")
	if b, err := ioutil.ReadFile("/proc/1/cgroup"); err != nil {
		// fmt.Println("read /proc/1/cgroup unsuccessful: ", err)
		return false
	} else {
		// fmt.Println("read /proc/1/cgroup successful, result: ", string(bytes))
		return strings.Contains(string(b), "docker") ||
			strings.Contains(string(b), "kubepods")
	}
}

// CreateAndExec creates a container and executes a command returning the output. If command is empty, it will return the output of the container.
func (u *DockerUtil) CreateAndExec(config container.Config, command string) (string, error) {
	createResp, err := u.docker.ContainerCreate(u.GoContext(), &config, nil, nil, nil, shortuuid.New()[:5])
	if err != nil {
		return "", errors.Wrapf(err, "failed to create container")
	}
	err = u.docker.ContainerStart(u.GoContext(), createResp.ID, types.ContainerStartOptions{})
	defer func(u *DockerUtil, containerID string, timeout time.Duration) {
		_ = u.ForceRemoveContainer(containerID, timeout)
	}(u, createResp.ID, 1*time.Second)
	if err != nil {
		return "", errors.Wrapf(err, "failed to start container")
	}
	if command == "" {
		if err := u.WaitUntilContainerExits(createResp.ID); err != nil {
			return "", errors.Wrapf(err, "failed to wait for container to exit")
		}
		reader, err := u.docker.ContainerLogs(u.GoContext(), createResp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
		if err != nil {
			return "", errors.Wrapf(err, "failed to read logs from container %s", createResp.ID)
		}
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(reader)
		if err != nil {
			return buf.String(), errors.Wrapf(err, "failed to read logs from container %s", createResp.ID)
		}
		return buf.String(), nil
	}
	return u.ExecInContainer(createResp.ID, command)
}

// CreateAndReadFile creates a container and reads a file from it.
func (u *DockerUtil) CreateAndReadFile(config container.Config, filePath string) (string, error) {
	createResp, err := u.docker.ContainerCreate(u.GoContext(), &config, nil, nil, nil, shortuuid.New()[:5])
	if err != nil {
		return "", errors.Wrapf(err, "failed to create container")
	}
	err = u.docker.ContainerStart(u.GoContext(), createResp.ID, types.ContainerStartOptions{})
	defer func(u *DockerUtil, containerID string, timeout time.Duration) {
		_ = u.ForceRemoveContainer(containerID, timeout)
	}(u, createResp.ID, 1*time.Second)
	if err != nil {
		return "", errors.Wrapf(err, "failed to start container")
	}
	return u.ReadFileFromContainer(createResp.ID, filePath)
}

// CreateAndCheckFileExists checks whether file exists within container
func (u *DockerUtil) CreateAndCheckFileExists(config container.Config, filePath string) (bool, error) {
	createResp, err := u.docker.ContainerCreate(u.GoContext(), &config, nil, nil, nil, shortuuid.New()[:5])
	if err != nil {
		return false, errors.Wrapf(err, "failed to create container")
	}
	err = u.docker.ContainerStart(u.GoContext(), createResp.ID, types.ContainerStartOptions{})
	defer func(u *DockerUtil, containerID string, timeout time.Duration) {
		_ = u.ForceRemoveContainer(containerID, timeout)
	}(u, createResp.ID, 1*time.Second)
	if err != nil {
		return false, errors.Wrapf(err, "failed to start container")
	}
	stat, err := u.docker.ContainerStatPath(u.GoContext(), createResp.ID, filePath)
	if err != nil {
		return false, err
	}
	return stat.Size != 0, nil
}

// DetectOSDistributionFromContainer detects which Linux distribution is running in Docker container
func (u *DockerUtil) DetectOSDistributionFromContainer(containerID string) OSDistribution {
	output, err := u.ReadFileFromContainer(containerID, "/etc/os-release")
	if err != nil {
		return UnknownOSDistribution
	}
	_, err = u.docker.ContainerStatPath(u.GoContext(), containerID, "/bin/sh")
	if err != nil {
		return UnknownOSDistribution
	}
	return OSReleaseOutputToDistribution(output)
}

// DetectOSDistributionFromImage detects which Linux distribution is running in Docker container
func (u *DockerUtil) DetectOSDistributionFromImage(image string) OSDistribution {
	createResp, err := u.docker.ContainerCreate(u.GoContext(), &container.Config{Image: image}, nil, nil, nil, shortuuid.New()[:5])
	if err != nil {
		return UnknownOSDistribution
	}
	containerID := createResp.ID
	err = u.docker.ContainerStart(u.GoContext(), containerID, types.ContainerStartOptions{})
	if err != nil {
		return UnknownOSDistribution
	}
	defer func(u *DockerUtil, containerID string, timeout time.Duration) {
		_ = u.ForceRemoveContainer(containerID, timeout)
	}(u, createResp.ID, 1*time.Second)
	return u.DetectOSDistributionFromContainer(containerID)
}

// IsDockerHostRemote returns true if Docker is running on different host
func (u *DockerUtil) IsDockerHostRemote() bool {
	dockerHost := u.DockerHost()
	// if DOCKER_HOST is specified and is not a unix socket, Docker is considered remote
	return dockerHost != "" && !strings.HasPrefix(strings.ToLower(dockerHost), "unix:")
}

// DockerHost returns DOCKER_HOST env variable (if it is set)
func (u *DockerUtil) DockerHost() string {
	return os.Getenv("DOCKER_HOST")
}

func ParsePortsSpecs(portSpecs []string) (map[nat.Port]struct{}, map[nat.Port][]nat.PortBinding, error) {
	var ports map[nat.Port]struct{}
	var portBindings map[nat.Port][]nat.PortBinding
	ports, portBindings, err := nat.ParsePortSpecs(portSpecs)
	// If simple port parsing fails try to parse as long format
	if err != nil {
		portSpecs, err = parsePortOpts(portSpecs)
		if err != nil {
			return nil, nil, err
		}
		ports, portBindings, err = nat.ParsePortSpecs(portSpecs)

		if err != nil {
			return nil, nil, err
		}
	}
	return ports, portBindings, nil
}

func parsePortOpts(publishOpts []string) ([]string, error) {
	var optsList []string
	for _, publish := range publishOpts {
		params := map[string]string{"protocol": "tcp"}
		for _, param := range strings.Split(publish, ",") {
			opt := strings.Split(param, "=")
			if len(opt) < 2 {
				return optsList, errors.Errorf("invalid publish opts format (should be name=value but got '%s')", param)
			}

			params[opt[0]] = opt[1]
		}
		optsList = append(optsList, fmt.Sprintf("%s:%s/%s", params["target"], params["published"], params["protocol"]))
	}
	return optsList, nil
}

var notDockerIdRegexp = regexp.MustCompile(`[^a-zA-Z0-9]`)

const MAX_DOCKER_ID_LENGTH = 24

func CleanupDockerID(someId string) string {
	cleanString := notDockerIdRegexp.ReplaceAllString(someId, "")
	if len(cleanString) < 1 {
		return cleanString
	}
	max := len(cleanString) - 1
	if max > MAX_DOCKER_ID_LENGTH {
		max = MAX_DOCKER_ID_LENGTH
	}
	return cleanString[:max]
}
