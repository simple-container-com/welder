package util

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"io"
	"strings"
)

func WaitForOutput(reader *io.PipeReader, onEof func(string)) *errgroup.Group {
	var eg errgroup.Group
	eg.Go(func() error {
		var buffer bytes.Buffer
		bufReader := bufio.NewReader(reader)
		for {
			raw, _, err := bufReader.ReadLine()
			buffer.Write(raw)
			switch err {
			case io.EOF:
				onEof(buffer.String())
				return nil
			case nil:
				fmt.Println(string(raw))
			default:
				if strings.Contains(err.Error(), "closed pipe") {
					onEof(buffer.String())
					return nil
				}
				return errors.Wrapf(err, "failed to read next line from build output")
			}
		}
	})
	return &eg
}
