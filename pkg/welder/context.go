package welder

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"

	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/mutagen"
	"github.com/simple-container-com/welder/pkg/util"
	"github.com/simple-container-com/welder/pkg/welder/types"
	"golang.org/x/sync/errgroup"
)

type BuildContext struct {
	*types.CommonCtx
}

// terminateExternalSyncSessions terminates any hanging sessions from current run
func (buildCtx *BuildContext) terminateExternalSyncSessions(runID string, params *runInContainerParams) error {
	syncer, err := mutagen.New(buildCtx.GoContext(), &util.NoopLogger{})
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen syncer")
	}
	allSessions, err := syncer.ListSessions()
	if err != nil {
		return err
	}
	sessions := make([]mutagen.SessionInfo, 0)
	for _, session := range allSessions {
		for _, volume := range params.volumes {
			if syncSessionName(runID, volume) == session.Name {
				sessions = append(sessions, session)
			}
		}
	}
	if err := buildCtx.waitForSessions(sessions, syncer); err != nil {
		return errors.Wrapf(err, "failed to wait until sessions complete")
	}
	for _, session := range sessions {
		if err := syncer.Terminate(session.SessionId); err != nil {
			return errors.Wrapf(err, "failed to terminate session %s", session.SessionId)
		}
	}
	return nil
}

// sync volumes to container using external tool (e.g. mutagen)
func (buildCtx *BuildContext) syncVolumesViaExternalTool(targetPrefix string, runID string, params *runInContainerParams) error {
	syncer, err := mutagen.New(buildCtx.GoContext(), &util.NoopLogger{})
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen syncer")
	}

	sessions := make([]mutagen.SessionInfo, 0)
	for _, vol := range params.volumes {
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
	}
	return buildCtx.waitForSessions(sessions, syncer)
}

func (buildCtx *BuildContext) waitForSessions(sessions []mutagen.SessionInfo, syncer *mutagen.Mutagen) error {
	errGroup := errgroup.Group{}
	for _, s := range sessions {
		session := s
		errGroup.Go(func() error {
			buildCtx.Logger().Logf("Waiting until session %s (%s) finishes sync...", session.Name, session.SessionId)
			return syncer.WaitForSyncToComplete(session.SessionId)
		})
	}
	return errGroup.Wait()
}

func syncSessionName(runID string, vol docker.Volume) string {
	return vol.NameOrPathToName(runID)
}

// CalcHash calculates hash sum of configuration
func (deployCtx *DeployContext) CalcHash() (string, error) {
	var b bytes.Buffer
	gob.Register(deployCtx)
	err := gob.NewEncoder(&b).Encode([]interface{}{deployCtx})
	return base64.StdEncoding.EncodeToString(b.Bytes()), err
}

type DeployContext struct {
	*BuildContext
	Envs []string
}

type MicrosCtx struct {
	*DeployContext
	detectedModule *types.ModuleDefinition
	rootDir        string
	cwd            string
	subPath        string
	root           types.RootBuildDefinition
	dockerResult   types.OutDockerDefinition
}
