package welder

import (
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/welder/runner"
	. "github.com/smecsia/welder/pkg/welder/types"
	"os"
)

// VolumesSync syncs all volumes to Docker volumes
func (buildCtx *BuildContext) VolumesSync(opts runner.SyncOpts) error {
	allVolumes := make([]docker.Volume, 0)

	detectedModule, root, err := ReadBuildModuleDefinition(buildCtx.RootDir())
	if err != nil {
		return err
	}
	run := runner.NewRun(buildCtx.CommonCtx)
	// calculate and merge all volumes of all active modules
	for _, module := range buildCtx.ActiveModules(root, detectedModule) {
		buildRunCtx, err := buildCtx.calcModuleBuildRunContext(&root, module, nil)
		if err != nil {
			return err
		}
		containerParams, err := run.CalcRunInContainerParams(root.ProjectNameOrDefault(), root.ConfiguredRootPath(), buildRunCtx.buildDef.CommonRunDefinition, &root.Default.MainVolume)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate run in container params")
		}
		for _, newVol := range containerParams.Volumes {
			fi, err := os.Stat(newVol.HostPath)
			if err != nil {
				return errors.Wrapf(err, "failed to stat volume %q", newVol.HostPath)
			}
			if !fi.IsDir() {
				buildCtx.Logger().Debugf("ignoring file volume %q", newVol.HostPath)
				continue
			}
			found := false
			for _, addedVol := range allVolumes {
				// if host path has been already added by another module/volume
				if addedVol.HostPath == newVol.HostPath {
					found = true
					break
				}
			}
			if !found {
				allVolumes = append(allVolumes, newVol)
			}
		}
	}
	return run.SyncVolumes(root, allVolumes, opts)
}
