/*
*
This entire package is a copy of Docker sources to publicize and use the ability to
run exec from within the command line seamlessly
*/
package dockerext

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	gosignal "os/signal"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/term"
	"golang.org/x/net/context"
)

// The default escape key sequence: ctrl-p, ctrl-q
var defaultEscapeKeys = []byte{16, 17}

type Streams struct {
	In  *InStream
	Out *OutStream
	Err io.Writer
}

// A HijackedIOStreamer handles copying input to and output from streams to the
// connection.
type HijackedIOStreamer struct {
	Streams      Streams
	InputStream  io.ReadCloser
	OutputStream io.Writer
	ErrorStream  io.Writer

	Resp types.HijackedResponse

	Tty        bool
	DetachKeys string
}

// stream handles setting up the IO and then begins streaming stdin/stdout
// to/from the hijacked connection, blocking until it is either done reading
// output, the user inputs the detach key sequence when in TTY mode, or when
// the given context is cancelled.
func (h *HijackedIOStreamer) Stream(ctx context.Context) error {
	restoreInput, err := h.setupInput()
	if err != nil {
		return fmt.Errorf("unable to setup input stream: %s", err)
	}

	defer restoreInput()

	outputDone := h.beginOutputStream(restoreInput)
	inputDone, detached := h.beginInputStream(restoreInput)

	select {
	case err := <-outputDone:
		return err
	case <-inputDone:
		// Input stream has closed.
		if h.OutputStream != nil || h.ErrorStream != nil {
			// Wait for output to complete streaming.
			select {
			case err := <-outputDone:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	case err := <-detached:
		// Got a detach key sequence.
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *HijackedIOStreamer) setupInput() (restore func(), err error) {
	if h.InputStream == nil || !h.Tty {
		// No need to setup input TTY.
		// The restore func is a nop.
		return func() {}, nil
	}

	if err := setRawTerminal(h.Streams); err != nil {
		return nil, fmt.Errorf("unable to set IO streams as raw terminal: %s", err)
	}

	// Use sync.Once so we may call restore multiple times but ensure we
	// only restore the terminal once.
	var restoreOnce sync.Once
	restore = func() {
		restoreOnce.Do(func() {
			_ = restoreTerminal(h.Streams, h.InputStream)
		})
	}

	// Wrap the input to detect detach escape sequence.
	// Use default escape keys if an invalid sequence is given.
	escapeKeys := defaultEscapeKeys
	if h.DetachKeys != "" {
		customEscapeKeys, err := term.ToBytes(h.DetachKeys)
		if err != nil {
			_ = fmt.Sprintf("invalid detach escape keys, using default: %s\n", err)
		} else {
			escapeKeys = customEscapeKeys
		}
	}

	h.InputStream = ioutils.NewReadCloserWrapper(term.NewEscapeProxy(h.InputStream, escapeKeys), h.InputStream.Close)

	return restore, nil
}

func (h *HijackedIOStreamer) beginOutputStream(restoreInput func()) <-chan error {
	if h.OutputStream == nil && h.ErrorStream == nil {
		// There is no need to copy output.
		return nil
	}

	outputDone := make(chan error)
	go func() {
		var err error

		// When TTY is ON, use regular copy
		if h.OutputStream != nil && h.Tty {
			_, err = io.Copy(h.OutputStream, h.Resp.Reader)
			// We should restore the terminal as soon as possible
			// once the connection ends so any following print
			// messages will be in normal type.
			restoreInput()
		} else {
			_, err = stdcopy.StdCopy(h.OutputStream, h.ErrorStream, h.Resp.Reader)
		}

		if err != nil {
			_ = fmt.Sprintf("Error receiveStdout: %s\n", err)
		}

		outputDone <- err
	}()

	return outputDone
}

func (h *HijackedIOStreamer) beginInputStream(restoreInput func()) (doneC <-chan struct{}, detachedC <-chan error) {
	inputDone := make(chan struct{})
	detached := make(chan error)

	go func() {
		if h.InputStream != nil {
			_, err := io.Copy(h.Resp.Conn, h.InputStream)
			// We should restore the terminal as soon as possible
			// once the connection ends so any following print
			// messages will be in normal type.
			restoreInput()

			if _, ok := err.(term.EscapeError); ok {
				detached <- err
				return
			}

			if err != nil {
				// This error will also occur on the receive
				// side (from stdout) where it will be
				// propagated back to the caller.
			}
		}

		if err := h.Resp.CloseWrite(); err != nil {
		}

		close(inputDone)
	}()

	return inputDone, detached
}

func setRawTerminal(streams Streams) error {
	if err := streams.In.SetRawTerminal(); err != nil {
		return err
	}
	return streams.Out.SetRawTerminal()
}

// nolint: unparam
func restoreTerminal(streams Streams, in io.Closer) error {
	streams.In.RestoreTerminal()
	streams.Out.RestoreTerminal()
	// WARNING: DO NOT REMOVE THE OS CHECKS !!!
	// For some reason this Close call blocks on darwin..
	// As the client exits right after, simply discard the close
	// until we find a better solution.
	//
	// This can also cause the client on Windows to get stuck in Win32 CloseHandle()
	// in some cases. See https://github.com/docker/docker/issues/28267#issuecomment-288237442
	// Tracked internally at Microsoft by VSO #11352156. In the
	// Windows case, you hit this if you are using the native/v2 console,
	// not the "legacy" console, and you start the client in a new window. eg
	// `start docker run --rm -it microsoft/nanoserver cmd /s /c echo foobar`
	// will hang. Remove start, and it won't repro.
	if in != nil && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return in.Close()
	}
	return nil
}

// MonitorTtySize updates the container tty size when the terminal tty changes size
func MonitorTtySize(ctx context.Context, client client.ContainerAPIClient, streams Streams, id string, isExec bool) error {
	resizeTty := func() {
		height, width := streams.Out.GetTtySize()
		resizeTtyTo(ctx, client, id, height, width, isExec)
	}

	resizeTty()

	if runtime.GOOS == "windows" {
		go func() {
			prevH, prevW := streams.Out.GetTtySize()
			for {
				time.Sleep(time.Millisecond * 250)
				h, w := streams.Out.GetTtySize()

				if prevW != w || prevH != h {
					resizeTty()
				}
				prevH = h
				prevW = w
			}
		}()
	} else {
		sigchan := make(chan os.Signal, 1)
		gosignal.Notify(sigchan, signal.SIGWINCH)
		go func() {
			for range sigchan {
				resizeTty()
			}
		}()
	}
	return nil
}

// resizeTtyTo resizes tty to specific height and width
func resizeTtyTo(ctx context.Context, client client.ContainerAPIClient, id string, height, width uint, isExec bool) {
	if height == 0 && width == 0 {
		return
	}

	options := types.ResizeOptions{
		Height: height,
		Width:  width,
	}

	var err error
	if isExec {
		err = client.ContainerExecResize(ctx, id, options)
	} else {
		err = client.ContainerResize(ctx, id, options)
	}

	if err != nil {
		fmt.Sprintf("Error resize: %s\n", err)
	}
}
