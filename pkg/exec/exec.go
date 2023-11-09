package exec

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/util"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// Exec defines execution on host environment
type Exec struct {
	logger     util.Logger
	context    context.Context
	output     *bytes.Buffer
	resEnvFile string
}

// ExecRes result of execution
type ExecRes struct {
	Pid int
	Env []string
}

// Opts execution options
type Opts struct {
	Wd  string
	Env []string
}

// NewExec initializes new host executor
func NewExec(context context.Context, logger util.Logger) Exec {
	return Exec{logger: logger, context: context}
}

// NewExecWithOutput initializes new host executor
func NewExecWithOutput(context context.Context, logger util.Logger, output *bytes.Buffer) Exec {
	res := NewExec(context, logger)
	res.output = output
	return res
}

// Lookup returns path to command
func Lookup(command string) (string, error) {
	return exec.LookPath(command)
}

// ExecCommandAndLog executes command and logs into the provided logger
func (e *Exec) ExecCommandAndLog(subject string, cmd string, opts Opts) (ExecRes, error) {
	res := ExecRes{}
	e.logger.Debugf("Executing %q", cmd)
	run := e.prepareCommand(cmd, opts)
	var eg errgroup.Group

	logReader, logWriter := io.Pipe()
	errlogReader, errlogWriter := io.Pipe()
	var out io.WriteCloser = logWriter
	var errOut io.WriteCloser = errlogWriter
	if e.output == nil {
		e.output = &bytes.Buffer{}
	}

	eg.Go(util.ReaderToLogFunc(logReader, false, "", e.logger, subject))
	eg.Go(util.ReaderToLogFunc(errlogReader, true, "ERR: ", e.logger, subject))
	captReader, captStdout := io.Pipe()
	eg.Go(util.ReaderToBufFunc(captReader, e.output))

	stdout := util.MultiWriteCloser(logWriter, captStdout)
	stderr := util.MultiWriteCloser(errlogWriter, captStdout)

	run.Stderr = stderr
	run.Stdout = stdout
	if err := run.Run(); err != nil {
		return res, err
	}
	res.Pid = run.ProcessState.Pid()
	res.Env = []string{}
	_, err := os.Stat(e.resEnvFile)
	if !os.IsNotExist(err) {
		if envFileBytes, err := os.ReadFile(e.resEnvFile); err == nil {
			res.Env = strings.Split(string(envFileBytes), "\n")
		}
	}
	if err := out.Close(); err != nil {
		return res, err
	}
	if err := errOut.Close(); err != nil {
		return res, err
	}
	if err := captStdout.Close(); err != nil {
		return res, err
	}
	return res, eg.Wait()
}

// ExecCommand executes command and returns output
func (e *Exec) ExecCommand(cmd string, opts Opts) (string, error) {
	e.logger.Debugf("Executing '%s'", cmd)
	run := e.prepareCommand(cmd, opts)
	res, err := run.CombinedOutput()
	return string(res), err
}

// ProxyExec executes command with all binding to parent process
func (e *Exec) ProxyExec(cmd string, opts Opts) error {
	e.logger.Debugf("Executing '%s'", cmd)
	run := e.prepareCommand(cmd, opts)
	run.Stdout = os.Stdout
	run.Stdin = os.Stdin
	run.Stderr = os.Stderr
	if err := run.Start(); err != nil {
		return errors.Wrapf(err, "failed to start command '%s'", cmd)
	}
	// wait for the command to finish
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- run.Wait()
		close(waitCh)
	}()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	// You need a for loop to handle multiple signals
	for {
		select {
		case sig := <-sigChan:
			if run.ProcessState == nil || run.ProcessState.Exited() {
				return nil
			}
			if err := run.Process.Signal(sig); err != nil {
				return errors.Wrapf(err, "error sending signal %s", sig)
			}
		case err := <-waitCh:
			// Subprocess exited. Get the return code, if we can
			var waitStatus syscall.WaitStatus
			if exitError, ok := err.(*exec.ExitError); ok {
				waitStatus = exitError.Sys().(syscall.WaitStatus)
				os.Exit(waitStatus.ExitStatus())
			}
			return err
		}
	}
	return nil
}

func (e *Exec) prepareCommand(cmd string, opts Opts) *exec.Cmd {
	e.resEnvFile = fmt.Sprintf("/tmp/%s.env", uuid.New().String())
	args := []string{"-c", fmt.Sprintf(`trap "env > %s" EXIT; %s`, e.resEnvFile, cmd)}
	run := exec.CommandContext(e.context, "sh", args...)
	if len(opts.Env) > 0 {
		run.Env = os.Environ()
		for _, env := range opts.Env {
			run.Env = append(run.Env, env)
		}
	}
	if opts.Wd != "" {
		run.Dir = opts.Wd
	}
	return run
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
