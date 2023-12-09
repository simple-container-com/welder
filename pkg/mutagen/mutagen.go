package mutagen

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"os/user"
	"path"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/exec"
	"github.com/simple-container-com/welder/pkg/render"
	"github.com/simple-container-com/welder/pkg/util"
)

const (
	mutagenSessionIdRegexp   = `(?P<SessionId>[_[:alnum:]-]+)`
	statusScanningFiles      = "Scanning files"
	statusWatchingForChanges = "Watching for changes"
	connectionDisconnected   = "Disconnected"
	connectionConnected      = "Connected"
)

var (
	sessionCreatedRegexp  = regexp.MustCompile(fmt.Sprintf(".*Created session\\s+%s", mutagenSessionIdRegexp))
	identifierRegexp      = regexp.MustCompile(fmt.Sprintf(".*Identifier:\\s+%s", mutagenSessionIdRegexp))
	dockerURLStringRegexp = regexp.MustCompile("\\s+URL:\\s+(?P<TargetURL>docker://(?P<Username>[_[:alnum:]-]+)@(?P<ContainerID>[^/]+)(?P<ContPath>[^\\n]+))")
)

type Mutagen struct {
	goContext  context.Context
	logger     util.Logger
	binaryPath string
}

type BaseInfo struct {
	SourcePath string
	TargetURL  string
	Labels     map[string]string
	Status     string
	AlphaState string
	BetaState  string
	Name       string
}

type SyncOpts struct {
	BaseInfo
	Monitor   bool
	SyncMode  string
	ExtraArgs string
}

type SessionInfo struct {
	BaseInfo
	SessionId   string
	Name        string
	ContainerID string
}

func New(ctx context.Context, logger util.Logger) (*Mutagen, error) {
	m := Mutagen{goContext: ctx, logger: logger}
	binaryPath, err := m.extractBinaryIfNecessary()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect path to mutagen binary")
	}
	m.logger.Debugf("path to mutagen has been found: %q", binaryPath)
	m.binaryPath = binaryPath
	return &m, nil
}

func (m *Mutagen) ListSessions() ([]SessionInfo, error) {
	cmd := exec.NewExec(m.goContext, m.logger)
	output, err := cmd.ExecCommand(fmt.Sprintf("%s sync list", m.binaryPath), exec.Opts{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list sessions, output: \n %s", output)
	}
	return ParseSessionsOutput(output)
}

func (m *Mutagen) Terminate(sessionId string) error {
	cmd := exec.NewExec(m.goContext, m.logger)
	output, err := cmd.ExecCommand(fmt.Sprintf("%s sync terminate %s", m.binaryPath, sessionId), exec.Opts{})
	if err != nil {
		return errors.Wrapf(err, "failed to terminate session, output: \n %s", output)
	}
	return nil
}

func ParseSessionsOutput(output string) ([]SessionInfo, error) {
	res := make([]SessionInfo, 0)
	lines := strings.Split(output, "\n")
	lineIdx := 0
	for lineIdx < len(lines) {
		if strings.HasPrefix(lines[lineIdx], "-----") { // new session
			session := SessionInfo{}
			lineIdx++
			if strings.HasPrefix(lines[lineIdx], "Name:") {
				session.Name = strings.Replace(lines[lineIdx], "Name: ", "", 1)
				lineIdx++
			}
			if strings.HasPrefix(lines[lineIdx], "Identifier:") {
				session.SessionId = strings.Replace(lines[lineIdx], "Identifier: ", "", 1)
				lineIdx++
			}
			if strings.HasPrefix(lines[lineIdx], "Labels:") {
				session.Labels = make(map[string]string)
				for lineIdx+1 < len(lines) && !strings.HasPrefix(lines[lineIdx+1], "Alpha:") {
					lineIdx++
					parts := strings.Split(lines[lineIdx], ":")
					key := strings.Trim(parts[0], "\t\n ")
					value := strings.Trim(parts[1], "\t\n ")
					session.Labels[key] = value
				}
				lineIdx++
			}
			if strings.HasPrefix(lines[lineIdx], "Alpha:") {
				for lineIdx+1 < len(lines) && !strings.HasPrefix(lines[lineIdx+1], "Beta:") {
					lineIdx++
					if strings.HasPrefix(lines[lineIdx], "\tURL:") {
						parts := strings.Split(lines[lineIdx], ":")
						session.SourcePath = strings.Trim(parts[1], " ")
					}
					if strings.HasPrefix(lines[lineIdx], "\tConnection state:") {
						session.AlphaState = strings.TrimSpace(strings.Replace(lines[lineIdx], "Connection state: ", "", 1))
					}
					if strings.HasPrefix(lines[lineIdx], "\tConnected:") {
						session.AlphaState = strings.TrimSpace(strings.Replace(lines[lineIdx], "Connected: ", "", 1))
						if session.AlphaState == "Yes" {
							session.AlphaState = "Connected"
						} else {
							session.AlphaState = "Disconnected"
						}
					}
				}
				lineIdx++
			}
			if strings.HasPrefix(lines[lineIdx], "Beta:") {
				for lineIdx+1 < len(lines) && !strings.HasPrefix(lines[lineIdx+1], "Status:") {
					lineIdx++
					if strings.HasPrefix(lines[lineIdx], "\tURL:") {
						if dockerURLStringRegexp.MatchString(lines[lineIdx]) {
							matches := MatchGroupsWithNames(dockerURLStringRegexp, lines[lineIdx])
							session.ContainerID = matches["ContainerID"]
							session.TargetURL = matches["TargetURL"]
						}
					}
					if strings.HasPrefix(lines[lineIdx], "\tConnection state:") {
						session.BetaState = strings.TrimSpace(strings.Replace(lines[lineIdx], "Connection state: ", "", 1))
					}
					if strings.HasPrefix(lines[lineIdx], "\tConnected:") {
						session.BetaState = strings.TrimSpace(strings.Replace(lines[lineIdx], "Connected: ", "", 1))
						if session.BetaState == "Yes" {
							session.BetaState = "Connected"
						} else {
							session.BetaState = "Disconnected"
						}
					}
				}
				lineIdx++
			}
			if strings.HasPrefix(lines[lineIdx], "Status:") {
				parts := strings.Split(lines[lineIdx], ":")
				session.Status = strings.Trim(parts[1], " ")
			}
			if session.SessionId != "" {
				res = append(res, session)
			}
		}
		lineIdx++
	}

	return res, nil
}

func (m *Mutagen) Status(sessionId string) (SessionInfo, error) {
	if sessions, err := m.ListSessions(); err != nil {
		return SessionInfo{}, err
	} else {
		for _, session := range sessions {
			if session.SessionId == sessionId {
				return session, nil
			}
		}
	}
	return SessionInfo{}, fmt.Errorf("session not found: %s", sessionId)
}

func (m *Mutagen) WaitForSyncToComplete(sessionId string) error {
	statusMsg := statusScanningFiles
	connection := connectionConnected
	for statusMsg != statusWatchingForChanges && connection == connectionConnected {
		time.Sleep(time.Second)
		if status, err := m.Status(sessionId); err != nil {
			return err
		} else {
			statusMsg = status.Status
			connection = status.BetaState
		}
	}
	return nil
}

func (m *Mutagen) RunCommand(command string) error {
	cmd := exec.NewExec(m.goContext, m.logger)

	err := cmd.ProxyExec(
		fmt.Sprintf("%s %s", m.binaryPath, command), exec.Opts{})
	if err != nil {
		return errors.Wrapf(err, "failed to exec command %q", command)
	}

	return nil
}

func (m *Mutagen) StartSync(opts SyncOpts) (SessionInfo, error) {
	cmd := exec.NewExec(m.goContext, m.logger)

	var extraOpts string
	if opts.SyncMode == "" {
		opts.SyncMode = "two-way-safe"
	}
	extraOpts = "--sync-mode " + opts.SyncMode
	if opts.Name != "" {
		extraOpts += fmt.Sprintf(" --name %s", opts.Name)
	}
	for k, v := range opts.Labels {
		extraOpts += fmt.Sprintf(" --label %s=%s", k, v)
	}
	extraOpts += " " + opts.ExtraArgs

	output, err := cmd.ExecCommand(
		fmt.Sprintf("%s sync create --name %s %s %s %s", m.binaryPath, opts.Name, opts.SourcePath, opts.TargetURL, extraOpts), exec.Opts{})
	if err != nil {
		return SessionInfo{}, errors.Wrapf(err, "failed to create sync for path %s, output: \n %s", opts.SourcePath, output)
	}

	if !strings.Contains(output, "Created session") {
		return SessionInfo{}, errors.Errorf("unexpected output from mutagen sync create: %s", output)
	}

	match := sessionCreatedRegexp.FindStringSubmatch(output)
	sessionId := match[1]

	// if we run in monitor mode, need to cleanup after termination
	if opts.Monitor {
		terminateFunc := func() error {
			cmd := exec.NewExec(context.Background(), m.logger)
			if output, err = cmd.ExecCommand(fmt.Sprintf("%s sync terminate %s", m.binaryPath, sessionId), exec.Opts{}); err != nil {
				m.logger.Logf("failed to terminate mutagen session %s: %s", sessionId, output)
				return err
			}
			return nil
		}
		m.addCallbackOnTermSignal(terminateFunc)
		defer terminateFunc()
		_, err = cmd.ExecCommandAndLog("sync",
			fmt.Sprintf("%s sync monitor %s", m.binaryPath, sessionId), exec.Opts{})
		if err != nil {
			return SessionInfo{}, errors.Wrapf(err, "failed to monitor mutagen for volume %s", opts.SourcePath)
		}
	} else if err := m.WaitForSyncToComplete(sessionId); err != nil {
		m.logger.Logf("Waiting until fully synced %s...", sessionId)
		return SessionInfo{}, errors.Wrapf(err, "failed to wait until complete")
	}
	return m.Status(sessionId)
}

func (m *Mutagen) extractBinaryIfNecessary() (string, error) {
	m.logger.Debugf("verifying we can run mutagen on host OS: %q", runtime.GOOS)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		return m.unzipMutagenToHomeDir()
	}
	return "", errors.Errorf("unsupported OS: %q", runtime.GOOS)
}

func (m *Mutagen) unzipMutagenToHomeDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get current user")
	}
	outputDir := path.Join(usr.HomeDir, ".mutagen", "bin")
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", errors.Wrapf(err, "failed to make sure mutagen dir exists")
	}
	m.logger.Debugf("created mutagen dir %s", outputDir)

	mutagenBinaryPath := path.Join(outputDir, "mutagen")
	if _, err := os.Stat(mutagenBinaryPath); err != nil && os.IsNotExist(err) {
		m.logger.Debugf("mutagen binary not found at %s, unzipping...", mutagenBinaryPath)

		zipFileName := fmt.Sprintf("mutagen-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
		mutagenZipPath := fmt.Sprintf("build/binaries/mutagen/%s", zipFileName)
		zipBytes, err := render.GetFile(mutagenZipPath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get zip bytes from asset %s", mutagenZipPath)
		}

		zipFilePath := path.Join(outputDir, zipFileName)
		m.logger.Debugf("writing mutagen to file %s", zipFilePath)
		err = ioutil.WriteFile(zipFilePath, zipBytes, os.ModePerm)

		if err != nil {
			return "", errors.Wrapf(err, "failed to write zip bytes from asset to file %s", zipFilePath)
		}

		m.logger.Debugf("unzipping mutagen from archive %s", zipFilePath)
		err = archiver.Unarchive(zipFilePath, outputDir)
		if err != nil {
			return "", errors.Wrapf(err, "failed to unarchive mutagen from zip file %s", zipFilePath)
		}
	}

	agentsFileName := "mutagen-agents.tar.gz"
	agentsFilePath := path.Join(outputDir, agentsFileName)
	if _, err := os.Stat(agentsFilePath); err != nil && os.IsNotExist(err) {
		m.logger.Debugf("mutagen library file not found at %s, extracting...", agentsFilePath)

		agentsBytes, err := render.GetFile(fmt.Sprintf("build/binaries/mutagen/%s", agentsFileName))
		if err != nil {
			return "", errors.Wrapf(err, "failed to get zip bytes from asset %s", agentsFileName)
		}

		m.logger.Debugf("writing %s to file %s", agentsFileName, agentsFilePath)
		err = ioutil.WriteFile(agentsFilePath, agentsBytes, os.ModePerm)

	}

	return mutagenBinaryPath, nil
}

func (m *Mutagen) addCallbackOnTermSignal(callback func() error) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGKILL)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGINT)

	go func() {
		<-signalCh
		err := callback()
		if err != nil {
			m.logger.Logf("error while cleanup: %s", err.Error())
		}
	}()
}
