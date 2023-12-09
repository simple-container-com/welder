package welder

import (
	"fmt"
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"
)

func IgnoreTestInitBitbucketPipelines(t *testing.T) {
	RegisterTestingT(t)

	rootDir, cleanup := createTempExampleProject(t, "testdata/example")
	defer cleanup()
	bbp := BitbucketPipelines{
		CommonCIGenerator: CommonCIGenerator{RootPath: rootDir},
		MainBranch:        "master",
	}

	err := bbp.Generate()
	Expect(err).To(BeNil())

	pipelinesFilePath := path.Join(rootDir, "bitbucket-pipelines.yml")
	assertFileExists(t, pipelinesFilePath)

	bytes, err := os.ReadFile(pipelinesFilePath)
	Expect(err).To(BeNil())
	fmt.Println(string(bytes))
	Expect(string(bytes)).To(MatchRegexp("" +
		"deployment: ddev[.\\s]+" +
		"name: Deploy trebuchet to ddev[.\\s]+" +
		"trigger: manual" +
		""))
	Expect(string(bytes)).To(MatchRegexp("" +
		"deployment: ddev[.\\s]+" +
		"name: Deploy armory to ddev[.\\s]+" +
		"trigger: automatic" +
		""))
	Expect(string(bytes)).To(MatchRegexp("" +
		"deployment: prod-east[.\\s]+" +
		"name: Deploy armory to prod-east[.\\s]+" +
		"trigger: manual" +
		""))
	Expect(string(bytes)).To(MatchRegexp("" +
		"deployment: stg-east[.\\s]+" +
		"name: Deploy trebuchet to stg-east[.\\s]+" +
		"trigger: manual" +
		""))
	Expect(string(bytes)).To(ContainSubstring("welder deploy --timestamps -e stg-west -m third"))
}
