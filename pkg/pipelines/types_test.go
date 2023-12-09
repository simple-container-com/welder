package pipelines_test

import (
	"testing"

	"github.com/simple-container-com/welder/pkg/pipelines"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func IgnoreTestPipelineUnmarshaling(t *testing.T) {
	pipelineYaml := []byte(`image: node:10.15.0
pipelines:
  default:
    - step:
        name: Build and test
        services:
          - redis
          - mysql
        script:
          - npm install
          - npm test
    - parallel:
        - step:
            name: Release
            script:
              - npm install
        - step:
            name: Publish
            script:
              - npm install
  tags:
    release-*:
      - step:
          name: Build and release
          script:
            - npm install
            - npm test
            - npm run release
  branches:
    staging:
      - step:
          name: Clone
          script:
            - echo "Clone all the things!"
  pull-requests:
    '**': #this runs as default for any branch not elsewhere defined
      - step:
          script:
            - ...
    feature/*: #any branch with a feature prefix
      - step:
          script:
            - ...
  custom: # Pipelines that are triggered manually
    sonar: # The name that is displayed in the list in the Bitbucket Cloud GUI
      - step:
          script:
            - echo "Manual triggers for Sonar are awesome!"
    deployment-to-prod: # Another display name
      - step:
          script:
            - echo "Manual triggers for deployments are awesome!"
definitions:
  services:
    redis:
      image: redis:3.2
    mysql:
      image: mysql:5.7
      variables:
        MYSQL_DATABASE: my-db
        MYSQL_ROOT_PASSWORD: $password
`)

	var pipeline pipelines.PipelineSpec

	err := yaml.UnmarshalStrict(pipelineYaml, &pipeline)

	require.NoError(t, err)
	assert.Equal(t, "Build and test", pipeline.Pipelines.Default[0].Steps[0].Name)
	assert.Equal(t, "Publish", pipeline.Pipelines.Default[1].Steps[1].Name)
}
