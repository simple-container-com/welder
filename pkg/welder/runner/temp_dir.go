package runner

import (
	"fmt"
	"github.com/smecsia/welder/pkg/docker"
	"os"
	"sync"
)

var (
	globalTempHostVolume     string
	globalTempHostVolumeSync sync.RWMutex
)

const globalTempHostVolumeEnv = "WELDER_TEMP_HOST_VOLUME"

func WelderTempDir() string {
	globalTempHostVolumeSync.Lock()
	defer globalTempHostVolumeSync.Unlock()

	if os.Getenv(globalTempHostVolumeEnv) != "" {
		globalTempHostVolume = os.Getenv(globalTempHostVolumeEnv)
	}

	if globalTempHostVolume == "" {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "welder")
		if err != nil {
			fmt.Println("Failed to create temp dir", err)
		}
		globalTempHostVolume = tmpDir
		_ = os.Setenv(globalTempHostVolumeEnv, globalTempHostVolume)
	}
	return globalTempHostVolume
}

func attachWelderTempDir(dockerRun *docker.Run, runParams *RunParams) {
	found := false
	for _, v := range runParams.Volumes {
		if v.HostPath == WelderTempDir() {
			found = true
		}
	}
	// append welder temp dir if it is not there
	if !found {
		runParams.Volumes = append(runParams.Volumes, docker.Volume{
			HostPath: WelderTempDir(),
			ContPath: WelderTempDir(),
			Mode:     docker.VolumeModeDelegated,
		})
	}
	// add env variable so that it is set for the container
	dockerRun.AddEnv(fmt.Sprintf("%s=%s", globalTempHostVolumeEnv, WelderTempDir()))
}
