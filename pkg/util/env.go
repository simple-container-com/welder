package util

import (
	"os"
	"strings"
)

func ToEnvVarName(key string) string {
	return strings.ReplaceAll(strings.ToUpper(key), "-", "_")
}

// CurrentCI represents the current CI environment.
type CurrentCI struct {
	Name string
}

func (ci *CurrentCI) IsRunningInBamboo() bool {
	return ci.Name == "bamboo" || (ci.Name == "" && len(os.Getenv("bamboo_JWT_TOKEN")) > 0)
}

func (ci *CurrentCI) IsRunningInBitbucketPipelines() bool {
	return ci.Name == "bitbucket-pipelines" || (ci.Name == "" && len(os.Getenv("PIPELINES_JWT_TOKEN")) > 0)
}
