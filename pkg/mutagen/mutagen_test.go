package mutagen_test

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/mutagen"
	"github.com/simple-container-com/welder/pkg/util"
)

func TestListSessions(t *testing.T) {
	RegisterTestingT(t)

	m, err := mutagen.New(context.Background(), util.NewStdoutLogger(os.Stdout, os.Stderr))
	Expect(err).To(BeNil())

	sessions, err := m.ListSessions()
	Expect(err).To(BeNil())
	Expect(sessions).NotTo(BeNil())
}

func TestParseSessionOutputNewFormat(t *testing.T) {
	RegisterTestingT(t)
	output := `
--------------------------------------------------------------------------------
Identifier: sync_jQQkLp36Cg2rMsXYNoqTR9kkBcuIaoSKNhOWIAy8Jtk
Alpha:
	URL: /home/someuser/projects/deng/welder/bin
	Connected: Yes
	Synchronizable contents:
		6 directories
		17 files (310 MB)
		0 symbolic links
Beta:
	URL: docker://someuser@sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976/tmp/test
	Connected: Yes
	Synchronizable contents:
		6 directories
		17 files (310 MB)
		0 symbolic links
Status: Watching for changes
--------------------------------------------------------------------------------
Identifier: sync_fdtRdOLx40RgSdaeplNOq0lBDUDpRvLvnyoVVfdTaqm
Alpha:
	URL: /home/someuser/projects/deng/welder/.welder-out
	Connected: Yes
	Synchronizable contents:
		5665 directories
		24070 files (573 MB)
		0 symbolic links
Beta:
	URL: docker://someuser@spinnaker-helm-chart-test-helm-hENDX/tmp/test
	Connected: Yes
	Synchronizable contents:
		0 directories
		0 files (0 B)
		0 symbolic links
Status: Staging files on beta
Staging progress: 196/24070 - 5.1 MB/573 MB - 1%
Current file: go/mod/cache/download/github.com/docker/docker/@v/v20.10.3+incompatible.zip (1.8 MB/4.6 MB)
--------------------------------------------------------------------------------
Identifier: sync_fdtRdOLxDISCONNECTED
Alpha:
	URL: /home/someuser/projects/deng/welder/
	Connected: Yes
Beta:
	URL: docker://root@alpine/tmp
	Connected: No
Last error: beta polling error: unable to receive poll response: unable to read message length: unexpected EOF
Status: Connecting to beta
--------------------------------------------------------------------------------

`
	sessions, err := mutagen.ParseSessionsOutput(output)

	Expect(err).To(BeNil())
	Expect(sessions).To(HaveLen(3))

	Expect(sessions[0].SessionId).To(Equal("sync_jQQkLp36Cg2rMsXYNoqTR9kkBcuIaoSKNhOWIAy8Jtk"))
	Expect(sessions[1].SessionId).To(Equal("sync_fdtRdOLx40RgSdaeplNOq0lBDUDpRvLvnyoVVfdTaqm"))
	Expect(sessions[1].AlphaState).To(Equal("Connected"))
	Expect(sessions[1].BetaState).To(Equal("Connected"))
	Expect(sessions[2].AlphaState).To(Equal("Connected"))
	Expect(sessions[2].BetaState).To(Equal("Disconnected"))
	Expect(sessions[0].ContainerID).To(Equal("sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976"))
	Expect(sessions[0].TargetURL).To(Equal("docker://someuser@sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976/tmp/test"))
	Expect(sessions[1].TargetURL).To(Equal("docker://someuser@spinnaker-helm-chart-test-helm-hENDX/tmp/test"))
	Expect(sessions[1].ContainerID).To(Equal("spinnaker-helm-chart-test-helm-hENDX"))
}

func TestParseSessionsOutput(t *testing.T) {
	RegisterTestingT(t)
	sessions, err := mutagen.ParseSessionsOutput(`
--------------------------------------------------------------------------------
Name: another-session-with-name-and-labels
Identifier: sync_uvL7cbti2SlhGZaTns7WeY0fqXQqUyeII8OdAuAYist
Labels:
	blah: value
	label2: value2
Alpha:
	URL: /tmp/test
	Connection state: Connected
Beta:
	URL: docker://someuser@sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976/tmp/test
		DOCKER_HOST=
		DOCKER_TLS_VERIFY=
		DOCKER_CERT_PATH=
	Connection state: Connected
Status: Watching for changes
--------------------------------------------------------------------------------
Name: TEST-NAME
Identifier: sync_JdPETCZllt1ekfISc3sb6fjPE3rmJrPHlJjPmM7Wqap
Labels: None
Alpha:
	URL: /tmp/test
	Connection state: Connected
Beta:
	URL: docker://someuser@spinnaker-helm-chart-test-helm-hENDX/tmp/test
		DOCKER_HOST=
		DOCKER_TLS_VERIFY=
		DOCKER_CERT_PATH=
	Connection state: Connected
Status: Watching for changes
--------------------------------------------------------------------------------
Identifier: sync_uasdhadjkasdk7297298374923NONAME
Labels: None
Alpha:
	URL: /tmp/test
	Connection state: Connected
Beta:
	URL: docker://someuser@sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976/tmp/test
		DOCKER_HOST=
		DOCKER_TLS_VERIFY=
		DOCKER_CERT_PATH=
	Connection state: Connected
Status: Watching for changes
--------------------------------------------------------------------------------
`)

	Expect(err).To(BeNil())
	Expect(sessions).To(HaveLen(3))

	Expect(sessions[0].SessionId).To(Equal("sync_uvL7cbti2SlhGZaTns7WeY0fqXQqUyeII8OdAuAYist"))
	Expect(sessions[1].SessionId).To(Equal("sync_JdPETCZllt1ekfISc3sb6fjPE3rmJrPHlJjPmM7Wqap"))
	Expect(sessions[2].SessionId).To(Equal("sync_uasdhadjkasdk7297298374923NONAME"))
	Expect(sessions[2].Status).To(Equal("Watching for changes"))
	Expect(sessions[2].AlphaState).To(Equal("Connected"))
	Expect(sessions[2].BetaState).To(Equal("Connected"))
	Expect(sessions[0].ContainerID).To(Equal("sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976"))
	Expect(sessions[0].TargetURL).To(Equal("docker://someuser@sha256:047a479d83031a40e16cbeb99e07d38a594b22f8913ec6a074bd5d0dd1607976/tmp/test"))
	Expect(sessions[1].TargetURL).To(Equal("docker://someuser@spinnaker-helm-chart-test-helm-hENDX/tmp/test"))
	Expect(sessions[1].ContainerID).To(Equal("spinnaker-helm-chart-test-helm-hENDX"))
}
