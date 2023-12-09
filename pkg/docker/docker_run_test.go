package docker_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	. "github.com/simple-container-com/welder/pkg/docker"
	"github.com/simple-container-com/welder/pkg/util"
)

func createTempDir(prefix string) (string, error) {
	if err := os.MkdirAll("testdata/tmp", os.ModePerm); err != nil {
		return "", err
	}
	if systemTmp, err := ioutil.TempDir("", prefix); err != nil {
		return "", err
	} else {
		testdataTmpDir, err := filepath.Abs(path.Join("testdata/tmp", path.Base(systemTmp)))
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(testdataTmpDir, os.ModePerm); err != nil {
			return "", err
		}
		return testdataTmpDir, nil
	}
}

func TestDockerRun(t *testing.T) {
	RegisterTestingT(t)

	cwd, err := os.Getwd()
	Expect(err).To(BeNil())

	tmpDir, err := createTempDir("tmpdir")
	tmpDir2, err := createTempDir("tmpdir2")
	defer os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir2)

	Expect(err).To(BeNil())

	dockerRun, err := NewRun("test123", "docker:latest")

	Expect(err).To(BeNil())

	dockerRun.FallbackToApproachInsteadOfBind(VolumeApproachAdd).SetVolumeBinds([]Volume{
		{
			HostPath: path.Join(cwd, "testdata"),
			ContPath: "/test",
		},
		{
			HostPath: path.Join(cwd, "testdata", "ValidDockerfile"),
			ContPath: "/some/valid/dockerfile/path",
		},
		{
			HostPath: tmpDir,
			ContPath: "/tmp/tempdir",
			Mode:     "rw",
		},
		{
			HostPath: tmpDir2,
			ContPath: "/tmp/tempdir2",
			Mode:     "rw",
		},
	}...)

	username := "mylongusername"

	reader, stdout := io.Pipe()

	var eg errgroup.Group

	eg.Go(func() error {
		var buffer bytes.Buffer
		bufReader := bufio.NewReader(reader)
		for {
			raw, _, err := bufReader.ReadLine()
			buffer.WriteString(string(raw) + "\n")
			switch err {
			case io.EOF:
				output := buffer.String()
				Expect(output).To(ContainSubstring("[docker / test123] > " + username))
				Expect(output).To(ContainSubstring("[docker / test123] > OK"))
				shortUsername := username[:8]
				Expect(output).To(MatchRegexp(fmt.Sprintf("\\[docker \\/ test123\\] > [rw\\-]+\\s+\\d\\s+%s\\s+%s\\s+\\d+\\s+\\w+\\s+\\d+\\s+[\\d:]+\\s+InvalidDockerfile", shortUsername, shortUsername)))
				Expect(output).To(MatchRegexp(fmt.Sprintf("\\[docker \\/ test123\\] > [rw\\-]+\\s+\\d\\s+%s\\s+%s\\s+\\d+\\s+\\w+\\s+\\d+\\s+[\\d:]+\\s+path", shortUsername, shortUsername)))

				outFilePath := path.Join(tmpDir, "somefile")
				_, err = os.Stat(outFilePath)
				Expect(os.IsNotExist(err)).To(Equal(false), "file "+outFilePath+" must exist")

				outFile2Path := path.Join(tmpDir2, "output", "somefile")
				_, err = os.Stat(outFile2Path)
				Expect(os.IsNotExist(err)).To(Equal(false), "file "+outFile2Path+" must exist")

				outFile2OutputPath := path.Join(tmpDir2, "output", "output", "somefile")
				_, err = os.Stat(outFile2OutputPath)
				Expect(os.IsNotExist(err)).To(Equal(true), "file "+outFile2OutputPath+" must not exist")

				return nil
			case nil:
				fmt.Println(string(raw))
			default:
				return errors.Wrapf(err, "failed to read next line from build output")
			}
		}
	})

	var out io.WriteCloser = stdout
	err = dockerRun.MountDockerSocket().Run(RunContext{
		Prefix: " \t [docker / test123] > ",
		Stdout: out, Stderr: out, User: username, Debug: true,
	},
		"whoami", "sh -c 'docker images && sleep 3 && ps -a && sleep 0.5 && ls -la /test'",
		"sh -c 'ls -la /some/valid/dockerfile && echo OK'",
		"echo 'BLAH' > /tmp/tempdir/somefile",
		"mkdir /tmp/tempdir2/output && echo 'BLAH2' > /tmp/tempdir2/output/somefile",
		"ls -la ~/.docker",
		"ls -la /var/run/docker.sock",
	)

	Expect(err).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}

func TestAllowReusingContainers(t *testing.T) {
	RegisterTestingT(t)
	dockerRun, _ := NewRun("reuse1234", "docker:latest")
	defer dockerRun.Destroy()
	err := dockerRun.AllowReuseContainers().Run(RunContext{Debug: true}, "echo 'I am still alive' > /tmp/alive")
	Expect(err).To(BeNil())

	// container must be alive by this point

	dockerRun, _ = NewRun("reuse1234", "docker:latest")
	err = dockerRun.AllowReuseContainers().Run(RunContext{Debug: true})
	Expect(err).To(BeNil())
	res, err := dockerRun.ExecWithOutput("cat /tmp/alive")
	Expect(err).To(BeNil())
	Expect(res).To(ContainSubstring("I am still alive"))
}

func TestVolumeApproachAdd(t *testing.T) {
	RegisterTestingT(t)
	dockerRun, _ := NewRun("volumeapproachadd123", "docker:latest")
	cwd, err := os.Getwd()
	Expect(err).To(BeNil())

	defer dockerRun.Destroy()
	err = dockerRun.
		SetVolumeApproach(VolumeApproachAdd).
		SetVolumeBinds([]Volume{
			{
				HostPath: path.Join(cwd, "testdata"),
				ContPath: "/test",
			},
			{
				HostPath: path.Join(cwd, "testdata", ".gitignore"),
				ContPath: "/other/.gitignore",
			},
		}...).Run(RunContext{Debug: true},
		"cat /test/.gitignore >> /test/tmp/output",
		"cat /other/.gitignore >> /test/tmp/output",
	)
	Expect(err).To(BeNil())

	outputFile := path.Join(cwd, "testdata", "tmp", "output")
	_, err = os.Stat(outputFile)
	Expect(os.IsNotExist(err)).To(Equal(false), "file "+outputFile+" must exist")
	outputFileBytes, _ := ioutil.ReadFile(outputFile)
	Expect(string(outputFileBytes)).To(ContainSubstring("tmptmp"))
}

func TestReturnsErrorWhenCommandFails(t *testing.T) {
	RegisterTestingT(t)
	dockerRun, _ := NewRun("test1234", "docker:latest")
	err := dockerRun.Run(RunContext{Debug: true, ErrorOnExitCode: true},
		"whoami", "exit 1",
	)
	Expect(err).NotTo(BeNil())
}

func TestWorkaroundForExistingSymlinksInContainer(t *testing.T) {
	RegisterTestingT(t)

	tmpDir, err := createTempDir("tmpdir")
	defer os.RemoveAll(tmpDir)
	srcFile, _ := os.Open("testdata/.gitignore")
	targetFile, _ := os.Create(path.Join(tmpDir, ".gitignore"))
	_, err = io.Copy(targetFile, srcFile)
	Expect(err).To(BeNil())

	dockerRun, err := NewRun("gradle-test", "gradle:latest")
	Expect(err).To(BeNil())
	dockerRun.SetVolumeBinds(Volume{
		HostPath: tmpDir,
		ContPath: "/root/.gradle",
		Mode:     "rw",
	}).FallbackToApproachInsteadOfBind(VolumeApproachCopy)

	reader, stdout := io.Pipe()

	eg := util.WaitForOutput(reader, func(output string) {
		outFilePath := path.Join(tmpDir, "test")
		_, err = os.Stat(outFilePath)
		Expect(os.IsNotExist(err)).To(Equal(false), "file "+outFilePath+" must exist")
		content, _ := ioutil.ReadFile(outFilePath)
		Expect(strings.TrimSpace(string(content))).To(Equal("OK"), "file "+outFilePath+" must contain 'OK'")
	})

	var out io.WriteCloser = stdout
	err = dockerRun.Run(RunContext{Stdout: out, Debug: true},
		"ls -la /root/.gradle", "echo 'OK' > /root/.gradle/test",
	)
	Expect(err).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
	Expect(err).To(BeNil())
}

func TestRunScratch(t *testing.T) {
	RegisterTestingT(t)

	reader, stdout := io.Pipe()
	defer reader.Close()
	dockerRun, err := NewRun("scrach-container-test", "hello-world")
	Expect(err).To(BeNil())
	eg := util.WaitForOutput(reader, func(output string) {
		Expect(output).To(ContainSubstring("Hello from Docker!This message shows that your installation appears to be working correctly"))
	})
	err = dockerRun.Run(RunContext{Debug: true, Stderr: stdout, Stdout: stdout})
	Expect(err).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}

func TestPreserveEnvVariables(t *testing.T) {
	RegisterTestingT(t)

	reader, stdout := io.Pipe()
	defer reader.Close()
	dockerRun, err := NewRun("preserve-env-variables", "docker:latest")
	Expect(err).To(BeNil())
	eg := util.WaitForOutput(reader, func(output string) {
		Expect(output).To(ContainSubstring("test-var=test"))
	})
	err = dockerRun.
		KeepEnvironmentWithEachCommand().
		Run(RunContext{Debug: true, Stderr: stdout, Stdout: stdout},
			"export TEST_VAR=test", "echo \"test-var=$TEST_VAR\"",
		)
	Expect(err).To(BeNil())
	Expect(eg.Wait()).To(BeNil())
}
