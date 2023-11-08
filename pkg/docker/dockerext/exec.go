package dockerext

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"
	"io"
)

type InteractiveExecCfg struct {
	Context    context.Context
	ExecConfig *types.ExecConfig
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.ReadCloser
	ExecID     string
}

func InteractiveExec(dockerAPI client.APIClient, cfg InteractiveExecCfg) error {
	// Interactive exec requested.
	execStartCheck := types.ExecStartCheck{
		Tty: cfg.ExecConfig.Tty,
	}
	resp, err := dockerAPI.ContainerExecAttach(cfg.Context, cfg.ExecID, execStartCheck)
	if err != nil {
		return err
	}
	defer resp.Close()

	streams := Streams{In: NewInStream(cfg.Stdin), Out: NewOutStream(cfg.Stdout), Err: cfg.Stderr}

	var eg errgroup.Group

	eg.Go(func() error {
		streamer := HijackedIOStreamer{
			Streams:      streams,
			InputStream:  cfg.Stdin,
			OutputStream: cfg.Stdout,
			ErrorStream:  cfg.Stderr,
			Resp:         resp,
			Tty:          cfg.ExecConfig.Tty,
			DetachKeys:   cfg.ExecConfig.DetachKeys,
		}

		return streamer.Stream(cfg.Context)
	})

	eg.Go(func() error {
		if cfg.ExecConfig.Tty && streams.In.IsTerminal() {
			if err := MonitorTtySize(cfg.Context, dockerAPI, streams, cfg.ExecID, true); err != nil {
				_, _ = fmt.Fprintln(streams.Err, "Error monitoring TTY size:", err)
			}
		}
		return nil
	})

	return eg.Wait()
}
