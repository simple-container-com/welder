package types

type Schema struct {
	SchemaVersion string `yaml:"schemaVersion"`
}

type ArtifactDefinition struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path,omitempty"`
}

type DeployModuleDefinition struct {
	RootPath        string                  `yaml:"rootPath,omitempty"`
	ModuleName      string                  `yaml:"moduleName,omitempty"`
	PreBuildCommand string                  `yaml:"preBuildCommand,omitempty"`
	DockerImages    []DockerImageDefinition `yaml:"dockerImages,omitempty"`
	Configuration   []ArtifactDefinition    `yaml:"configuration,omitempty"`
}

type DeployRootDefinition struct {
	Schema           `yaml:",inline"`
	ServiceName      string                   `yaml:"serviceName,omitempty"`
	RelativePathToSD string                   `yaml:"relativePathToSD,omitempty"`
	GitRoot          string                   `yaml:"gitRoot,omitempty"`
	PreBuildCommand  string                   `yaml:"preBuildCommand,omitempty"`
	Modules          []DeployModuleDefinition `yaml:"modules,omitempty"`
}

type DeployModulePointer struct {
	Schema       `yaml:",inline"`
	PathToRoot   string `yaml:"pathToRoot,omitempty"`
	PathFromRoot string `yaml:"pathFromRoot,omitempty"`
}

type ReleaseGroupDefinition struct {
	Env       string `yaml:"env,omitempty"`
	Region    string `yaml:"region,omitempty"`
	Label     string `yaml:"label,omitempty"`
	Account   string `yaml:"account,omitempty"`
	ReleaseId string `yaml:"releaseId,omitempty"`
}

type ReleaseDefinition struct {
	SchemaVersion string                            `yaml:"schemaVersion,omitempty"`
	ServiceName   string                            `yaml:"serviceName,omitempty"`
	ReleaseGroup  string                            `yaml:"releaseGroup,omitempty"`
	Releases      map[string]ReleaseGroupDefinition `yaml:"releases,omitempty"`
}
