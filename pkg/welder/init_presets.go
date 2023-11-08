package welder

import "github.com/smecsia/welder/pkg/welder/types"

type BuildPreset struct {
	Image       string
	Command     string
	Volumes     types.VolumesDefinition
	Env         types.BuildEnv
	DefaultArgs types.BuildArgs
	Profiles    map[string]types.ProfileDefinition
}

var (
	BuildInitPresets = map[string]BuildPreset{
		"maven": {
			Image:   "maven:${arg:maven-version}",
			Command: "mvn clean package ${BUILD_ARGS}",
			Volumes: types.VolumesDefinition{
				"~/.m2/settings.xml:${container:home}/.m2/settings.xml:ro",
				"~/.m2/settings-security.xml:${container:home}/.m2/settings-security.xml:ro",
				"~/.m2/repository:${container:home}/.m2/repository",
			},
			DefaultArgs: types.BuildArgs{
				"maven-version": "3.6-jdk-8-slim",
			},
			Profiles: types.ProfilesDefinition{
				"skip-tests": {
					BasicModuleDefinition: types.BasicModuleDefinition{Build: types.BuildDefinition{
						CommonRunDefinition: types.CommonRunDefinition{
							CommonSimpleRunDefinition: types.CommonSimpleRunDefinition{
								Env: types.BuildEnv{
									"BUILD_ARGS": "${env:BUILD_ARGS} -DskipTests",
								},
							},
						},
					}},
				},
			},
		},
		"rust": {
			Image:   "rust:${arg:rust-version}",
			Command: "make build",
			DefaultArgs: types.BuildArgs{
				"rust-version": "1.33",
			},
		},
		"gradle": {
			Image:   "gradle:${arg:gradle-version}",
			Command: "./gradlew clean build",
			Volumes: types.VolumesDefinition{
				"~/.gradle/init.gradle:${container:home}/.gradle/init.gradle:ro",
				"~/.gralde/caches:${container:home}/.gradle/caches",
			},
			DefaultArgs: types.BuildArgs{
				"gradle-version": "5.2.1-jdk8",
			},
		},
		"npm": {
			Image:   "mhart/alpine-node:${arg:node-version}",
			Command: "npm install && npm build",
			Volumes: types.VolumesDefinition{
				"~/.m2/.npmrc:${container:home}/.npmrc",
			},
			DefaultArgs: types.BuildArgs{
				"node-version": "10",
			},
		},
		"yarn": {
			Image:   "mhart/alpine-node:${arg:node-version}",
			Command: "yarn install && yarn build",
			Volumes: types.VolumesDefinition{
				"~/.m2/.npmrc:${container:home}/.npmrc",
			},
			DefaultArgs: types.BuildArgs{
				"node-version": "10",
			},
		},
	}
)
