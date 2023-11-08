package docker_test

import (
	"encoding/json"
	. "github.com/onsi/gomega"
	. "github.com/smecsia/welder/pkg/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestDockerImageFromReference(t *testing.T) {
	RegisterTestingT(t)

	img, err := ImageFromReference("docker.simple-container.com/deng/test/ok:blah1")
	Expect(err).To(BeNil())

	Expect(img).To(Equal("docker.simple-container.com/deng/test/ok"))
}

func TestResolveImageWithTag(t *testing.T) {
	image, err := ResolveDockerImageReference("ubuntu")

	require.NoError(t, err)

	assert.Equal(t, "index.docker.io/library/ubuntu", image.Name)
	assert.Regexp(t, "ubuntu@sha256:[0-9A-Fa-f]{32,}", image.Reference)
	assert.Regexp(t, "sha256:[0-9A-Fa-f]{32,}", image.Digest)
}

func TestResolveImageWithDigest(t *testing.T) {
	image, err := ResolveDockerImageReference("ubuntu@sha256:de774a3145f7ca4f0bd144c7d4ffb2931e06634f11529653b23eba85aef8e378")

	require.NoError(t, err)

	assert.Equal(t, "index.docker.io/library/ubuntu", image.Name)
	assert.Regexp(t, "ubuntu@sha256:[0-9A-Fa-f]{32,}", image.Reference)
	assert.Regexp(t, "sha256:[0-9A-Fa-f]{32,}", image.Digest)
}

func TestResolveNotExistingImage(t *testing.T) {
	_, err := ResolveDockerImageReference("not-existing-user/some-not-existing-image1234567890")

	require.Error(t, err)
}

func TestResolveExternalAuths(t *testing.T) {
	homeCfg, err := ReadDockerConfigJson()
	require.NoError(t, err)

	err = homeCfg.ResolveExternalAuths()
	require.NoError(t, err)

	tmpFile, err := homeCfg.DumpToTmpFile()
	require.NoError(t, err)

	configJsonBytes, err := ioutil.ReadFile(tmpFile)
	require.NoError(t, err)

	err = json.Unmarshal(configJsonBytes, &homeCfg)
	require.NoError(t, err)
}
