package welder

import (
	"fmt"

	"github.com/pkg/errors"
)

type BitbucketPipelines struct {
	CommonCIGenerator

	MainBranch string
}

func (o *BitbucketPipelines) Init() error {
	return nil
}

func (o *BitbucketPipelines) Generate() error {
	o.InitFiles = map[string]string{
		"bitbucket-pipelines.yml.tpl": "bitbucket-pipelines.yml",
	}
	o.AssetsBaseDir = "bitbucket-pipelines/"

	if _, err := o.CommonCIGenerator.Init(); err != nil {
		return errors.Wrapf(err, "failed to init template")
	}
	if err := o.CommonCIGenerator.Generate(o); err != nil {
		return errors.Wrapf(err, "failed to generate build system files")
	}

	fmt.Println("Bitbucket Pipelines has been successfully generated")

	return nil
}
