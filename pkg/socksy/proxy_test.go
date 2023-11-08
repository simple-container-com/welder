package socksy

import (
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	"golang.org/x/sync/errgroup"
	"net"
	"os"
	"testing"
	"time"
)

func TestToEnvVarName(t *testing.T) {
	RegisterTestingT(t)

	unixSocketPath := "/tmp/some-unix-socket.sock"
	Expect(os.RemoveAll(unixSocketPath)).To(BeNil())
	Expect(os.MkdirAll(unixSocketPath, os.ModePerm)).To(BeNil())

	proxy := NewUnixSocketProxy(unixSocketPath)

	port, err := proxy.Start(0, util.NewStdoutLogger(os.Stdout, os.Stderr))

	Expect(port).NotTo(BeNil())
	Expect(err).To(BeNil())

	var eg errgroup.Group
	eg.Go(func() error {
		return startEchoSocket(t, unixSocketPath, func(s string) {
			Expect(s).To(Equal("something"))
		})
	})
	time.Sleep(300 * time.Millisecond)
	eg.Go(func() error {
		return writeToAddr(t, fmt.Sprintf("localhost:%d", port), "something")
	})

	Expect(eg.Wait()).To(BeNil())
}

func startEchoSocket(t *testing.T, pathToSocket string, callback func(string)) error {
	if err := os.RemoveAll(pathToSocket); err != nil {
		return err
	}
	l, err := net.Listen("unix", pathToSocket)
	if err != nil {
		return err
	}
	fd, err := l.Accept()
	if err != nil {
		t.Fatal("failed to accept socket", err)
	}
	echoServer(t, fd, callback)
	return nil
}

func echoServer(t *testing.T, c net.Conn, callback func(string)) {
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			return
		}
		data := buf[0:nr]
		println("Server got:", string(data))
		callback(string(data))
		if nr == 0 {
			return
		}
	}
}

func writeToAddr(t *testing.T, addr string, value string) error {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal("failed to connect to TCP", err)
	}
	defer c.Close()
	_, err = c.Write([]byte(value))
	if err != nil {
		return err
	}
	return nil
}
