package types_test

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	dsl "github.com/smecsia/welder/pkg/welder/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestArgsToMap(t *testing.T) {
	dockerBuild := dsl.DockerBuildDefinition{
		Args: []dsl.DockerBuildArg{
			{Name: "someArg", Value: "someVal"},
			{Name: "fileArg", File: "testdata/testFileArg"},
		},
	}

	argsMap, err := dockerBuild.ArgsToMap()

	require.NoError(t, err)

	assert.Equal(t, "someVal", *argsMap["someArg"])
	assert.Equal(t, "testFileContent\nasd\n", *argsMap["fileArg"])
}

func TestBuildContextHash(t *testing.T) {
	RegisterTestingT(t)

	ctx1 := dsl.NewCommonContext(&dsl.CommonCtx{
		BuildArgs: dsl.BuildArgs{"blah": "blah"},
	}, &util.NoopLogger{})
	ctx2 := dsl.NewCommonContext(&dsl.CommonCtx{
		BuildArgs: dsl.BuildArgs{"blah": "blah2"},
	}, &util.NoopLogger{})
	ctx3 := dsl.NewCommonContext(&dsl.CommonCtx{
		BuildArgs: dsl.BuildArgs{"blah": "blah"},
	}, &util.NoopLogger{})
	hash1, err := ctx1.CalcHash()
	Expect(err).To(BeNil())

	hash2, err := ctx2.CalcHash()
	Expect(err).To(BeNil())

	hash3, err := ctx3.CalcHash()
	Expect(err).To(BeNil())

	Expect(hash1).NotTo(Equal(hash2))
	Expect(hash1).To(Equal(hash3))
}
