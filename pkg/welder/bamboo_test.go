package welder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func IgnoreTestInitBambooSpecs(t *testing.T) {
	bambooSpecs := setup(t)

	rootDir, err := os.Getwd()
	require.NoError(t, err)

	defer cleanup(t, bambooSpecs)

	err = bambooSpecs.Generate()
	require.NoError(t, err)

	err = os.Chdir(rootDir)
	require.NoError(t, err)

	assertFileExists(t, path.Join(bambooSpecs.RootPath, "bamboo-specs", "pom.xml"))
	specsFilePath := path.Join(bambooSpecs.RootPath, "bamboo-specs/src/main/kotlin/com/atlassian/ptl", "BambooSpecs.kt")
	assertFileExists(t, specsFilePath)

	bytes, err := ioutil.ReadFile(specsFilePath)
	require.NoError(t, err)
	fmt.Println(string(bytes))
	assert.Regexp(t, "Location\\(\\s+ name = \"ddev\",\\s+autoDeployed = true\\s+\\),\\s+"+
		"Location\\(\\s+name = \"prod-east\",\\s+autoDeployed = false\\s+\\),\\s+"+
		"Location\\(\\s+name = \"stg-east\",\\s+autoDeployed = false\\s+\\)\\s+\\),", string(bytes))
}

func assertFileExists(t *testing.T, filePath string) {
	_, err := os.Stat(filePath)
	assert.False(t, os.IsNotExist(err))
}

func cleanup(t *testing.T, bambooSpecs BambooSpecs) {
	err := os.RemoveAll(bambooSpecs.RootPath)
	require.NoError(t, err)
}

func setup(t *testing.T) BambooSpecs {
	depDir, cleanup := createTempExampleProject(t, "testdata/example")
	defer cleanup()
	return BambooSpecs{
		DirectoryName:     "bamboo-specs",
		CommonCIGenerator: CommonCIGenerator{RootPath: depDir},
		PackageName:       "com.atlassian.ptl",
	}
}
