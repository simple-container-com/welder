package runner

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/docker"
	"github.com/smecsia/welder/pkg/mutagen"
	"github.com/smecsia/welder/pkg/util"
	"github.com/smecsia/welder/pkg/welder/types"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"strings"
)

type SyncOpts struct {
	Recreate     bool
	Watch        bool
	ExtraOptions string
}

const (
	AtlasBuildVolumeSrc    = "AtlasBuildVolumeSrc"
	AtlasBuildVolumeTarget = "AtlasBuildVolumeTarget"
)

func (ctx *Run) ConfigureVolumes(dockerRun *docker.Run, runParams *RunParams) error {
	if ctx.SyncMode == "" {
		ctx.SyncMode = types.SyncModeBind // default is bind
	}

	switch ctx.SyncMode {
	// when bind or copy mode is enabled, fallback to copy mode automatically (if running in dind)
	case types.SyncModeBind:
		dockerRun.
			FallbackToApproachInsteadOfBind(docker.VolumeApproachCopy).
			SetVolumeBinds(runParams.Volumes...)
		return nil
	// when add mode is enabled
	case types.SyncModeAdd:
		dockerRun.
			SetVolumeApproach(docker.VolumeApproachAdd).
			SetVolumeBinds(runParams.Volumes...)
		return nil
	// when copy mode is enabled
	case types.SyncModeCopy:
		dockerRun.
			SetVolumeApproach(docker.VolumeApproachCopy).
			SetVolumeBinds(runParams.Volumes...)
		return nil
	// do not create bind volumes if they are supposed to be synced by external tool
	case types.SyncModeExternal:
		dockerRun.
			SetVolumeApproach(docker.VolumeApproachExternal).
			SetVolumeBinds(runParams.Volumes...)
		return nil
	case types.SyncModeVolume:
		// if reuse volumes mode is enabled, convert binds to volume mounts
		volumeMounts := make([]docker.Volume, 0)
		dockerUtil, err := docker.NewDefaultUtil(ctx.GoContext())
		if err != nil {
			return errors.Wrapf(err, "failed to init docker util")
		}
		for _, v := range runParams.Volumes {
			fi, err := os.Stat(v.HostPath)
			if err != nil {
				return errors.Wrapf(err, "failed to stat volume %q", v.HostPath)
			}
			// adding bind volume if it is a file
			if !fi.IsDir() {
				dockerRun.AddVolumeBinds(v)
				continue
			}
			// convert to volume mount if it is a directory
			volumeName := v.NameOrPathToName(runParams.ProjectName)
			volumeMounts = append(volumeMounts, docker.Volume{
				Name:     volumeName,
				ContPath: v.ContPath,
				Mode:     v.Mode,
			})
			if !dockerUtil.VolumeExists(volumeName) {
				return errors.Errorf("volume does not exist: %s (for host path %s). Make sure you've created it with 'volume' command", volumeName, v.HostPath)
			}
		}
		dockerRun.SetVolumeMounts(volumeMounts...)
		return nil
	}

	return errors.Errorf("unknown sync mode specified: %s", ctx.SyncMode)

}

// SyncVolumesViaExternalTool sync volumes to container using external tool (e.g. mutagen)
func (ctx *Run) SyncVolumesViaExternalTool(targetPrefix string, runID string, params *RunParams) error {
	syncer, err := mutagen.New(ctx.GoContext(), &util.NoopLogger{})
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen syncer")
	}

	sessions := make([]mutagen.SessionInfo, 0)
	ids := make([]string, 0)
	for _, vol := range params.Volumes {
		syncMode := "one-way-safe"
		if vol.Mode.IsRW() {
			syncMode = "two-way-safe"
		}
		targetURL := fmt.Sprintf("%s%s", targetPrefix, vol.ContPath)
		session, err := syncer.StartSync(mutagen.SyncOpts{
			Monitor:  false,
			SyncMode: syncMode,
			BaseInfo: mutagen.BaseInfo{
				Name:       syncSessionName(runID, vol),
				SourcePath: vol.HostPath,
				TargetURL:  targetURL,
			},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to start mutagen sync session for %s", vol.HostPath)
		}
		sessions = append(sessions, session)
		ids = append(ids, session.SessionId)
	}
	ctx.Logger().Debugf("Waiting for mutagen sync sessions (%s) to complete...", strings.Join(ids, ","))
	return ctx.waitForSessions(sessions, syncer)
}

// terminateExternalSyncSessions terminates any hanging sessions from current run
func (ctx *Run) terminateExternalSyncSessions(runID string, params *RunParams) error {
	syncer, err := mutagen.New(ctx.GoContext(), &util.NoopLogger{})
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen syncer")
	}
	allSessions, err := syncer.ListSessions()
	if err != nil {
		return err
	}
	sessions := make([]mutagen.SessionInfo, 0)
	for _, session := range allSessions {
		for _, volume := range params.Volumes {
			if syncSessionName(runID, volume) == session.Name {
				sessions = append(sessions, session)
			}
		}
	}
	if err := ctx.waitForSessions(sessions, syncer); err != nil {
		return errors.Wrapf(err, "failed to wait until sessions complete")
	}
	for _, session := range sessions {
		if err := syncer.Terminate(session.SessionId); err != nil {
			return errors.Wrapf(err, "failed to terminate session %s", session.SessionId)
		}
	}
	return nil
}

func (ctx *Run) waitForSessions(sessions []mutagen.SessionInfo, syncer *mutagen.Mutagen) error {
	errGroup := errgroup.Group{}
	for _, s := range sessions {
		session := s
		errGroup.Go(func() error {
			ctx.Logger().Logf("Waiting until session %s (%s) finishes sync...", session.Name, session.SessionId)
			return syncer.WaitForSyncToComplete(session.SessionId)
		})
	}
	return errGroup.Wait()
}

func syncSessionName(runID string, vol docker.Volume) string {
	return vol.NameOrPathToName(runID)
}

// SyncVolumes syncs all volumes to Docker volumes
func (ctx *Run) SyncVolumes(root types.RootBuildDefinition, volumes []docker.Volume, opts SyncOpts) error {
	if ctx.Parallel {
		ctx.Logger().Logf(" - Running in parallel with max: %d", ctx.ParallelCount)
	}
	for _, v := range volumes {
		volume := v
		ctx.Logger().Logf(" - Start syncing volume: %s", volume.HostPath)
		subCtx := types.NewCommonContext(ctx.CommonCtx, ctx.Logger().SubLogger("..."+util.LastNChars(volume.HostPath, 20)))
		run := NewRun(subCtx)
		doSync := func() error {
			err := run.SyncVolume(root.ProjectNameOrDefault(), volume, opts)
			if err != nil {
				ctx.Cancel("failed to sync volume %s", volume.HostPath)
				return errors.Wrapf(err, "failed to synchronize volume %q", volume.HostPath)
			}
			return nil
		}
		if err := ctx.StartParallel(doSync); err != nil {
			return errors.Wrapf(err, "failed to start sync for volume %q", volume.HostPath)
		}
	}
	return ctx.WaitParallel()
}

// SyncVolume runs sync process for the specific volume
func (ctx *Run) SyncVolume(projectName string, volume docker.Volume, opts SyncOpts) error {
	var eg errgroup.Group
	ctx.Logger().Debugf("Syncing volume %q -> %q", volume.HostPath, volume.ContPath)
	volumeName := volume.NameOrPathToName(projectName)
	ctx.Logger().Debugf("Volume name will be %q", volumeName)
	run, err := docker.NewRun(volumeName, "alpine:latest")

	defer func(run *docker.Run) {
		_ = run.Destroy()
	}(run)

	if err != nil {
		return errors.Wrapf(err, "failed to init docker run for mutagen")
	}

	reader, stdout := io.Pipe()
	readerErr, stderr := io.Pipe()
	var out io.WriteCloser = stdout
	eg.Go(util.ReaderToLogFunc(reader, false, "", ctx.Logger(), "sync"))
	eg.Go(util.ReaderToLogFunc(readerErr, true, "ERR: ", ctx.Logger(), "sync"))
	run.
		SetVolumeMounts(docker.Volume{
			Name:     volumeName,
			ContPath: volume.ContPath,
			Mode:     docker.VolumeModeDelegated,
		}).
		SetContext(ctx.GoContext()).
		SetCommand("-c", "while sleep 100000; do :; done").
		SetContext(ctx.GoContext()).
		SetEntrypoint("/bin/sh")
	if ctx.ReuseContainers {
		run.AllowReuseContainers()
	}
	if ctx.RemoveOrphans {
		run.EnableCleanupOrphans()
	}

	if opts.Recreate {
		if err := run.Destroy(); err != nil {
			return errors.Wrapf(err, "failed to cleanup orphan containers")
		}
		if run.Util().VolumeExists(volumeName) {
			err := run.Util().VolumeRemove(volumeName)
			if err != nil {
				return errors.Wrapf(err, "failed to remove volume %q", volumeName)
			}
		}
		if err = run.Util().CreateVolume(volumeName,
			map[string]string{AtlasBuildVolumeSrc: volume.HostPath, AtlasBuildVolumeTarget: volume.ContPath}); err != nil {
			return errors.Wrapf(err, "failed to create volume")
		}
	}

	err = run.
		Run(docker.RunContext{
			Detached: true,
			User:     ctx.Username,
			Debug:    ctx.Verbose,
			WorkDir:  volume.ContPath,
			Stdout:   out,
			Stderr:   stderr,
			Logger:   ctx.Logger(),
		})
	if err != nil {
		return errors.Wrapf(err, "failed to start mutagen container for volume %q", volume.HostPath)
	}

	defer func(run *docker.Run) {
		if err := run.Destroy(); err != nil {
			ctx.Logger().Errf("Failed to cleanup mutagen container for volume %q: %v", volume.HostPath, err)
		}
	}(run)

	mutagenProc, err := mutagen.New(ctx.GoContext(), ctx.Logger())
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen")
	}

	targetURL := fmt.Sprintf("docker://%s@%s%s", ctx.Username, run.ContainerID(), volume.ContPath)
	_, err = mutagenProc.StartSync(mutagen.SyncOpts{
		BaseInfo: mutagen.BaseInfo{
			Name:       volumeName,
			SourcePath: volume.HostPath,
			TargetURL:  targetURL,
		},
		Monitor: opts.Watch,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to start sync session")
	}

	return eg.Wait()
}
