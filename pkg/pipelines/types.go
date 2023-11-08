package pipelines

type PipelineSpec struct {
	Image       string             `yaml:"image,omitempty"`
	Pipelines   PipelineDefinition `yaml:"pipelines,omitempty"`
	Definitions ResourceSection    `yaml:"definitions,omitempty"`
	Options     OptionsSection     `yaml:"options,omitempty"`
	Clone       interface{}        `yaml:"clone,omitempty"`
}

type PipelineDefinition struct {
	Default     []PipelineStage            `yaml:"default,omitempty"`
	Branches    map[string][]PipelineStage `yaml:"branches,omitempty"`
	Tags        map[string][]PipelineStage `yaml:"tags,omitempty"`
	Bookmarks   map[string][]PipelineStage `yaml:"bookmarks,omitempty"`
	Custom      map[string][]PipelineStage `yaml:"custom,omitempty"`
	PullRequest map[string][]PipelineStage `yaml:"pull-requests,omitempty"`
}

type ResourceSection struct {
	Services map[string]ServiceDefiniton `yaml:"services,omitempty"`
	Caches   map[string]string           `yaml:"caches,omitempty"`
}

type OptionsSection struct {
	MaxTime int `yaml:"max-time,omitempty"`
}

type PipelineStage struct {
	Steps []PipelineStep
}

type SinglePipelineDefinition struct {
	SingleStep PipelineStep `yaml:"step,omitempty"`
}

type ParallelPipelineDefinition struct {
	ParallelSteps []SinglePipelineDefinition `yaml:"parallel,omitempty"`
}

type PipelineStep struct {
	Name  string `yaml:"name,omitempty"`
	Image string `yaml:"image,omitempty"`

	Script      []string `yaml:"script,omitempty"`
	AfterScript []string `yaml:"after-script,omitempty"`

	Artifacts []string `yaml:"artifacts,omitempty"`

	Services []string `yaml:"services,omitempty"`

	MaxTime int `yaml:"max-time,omitempty"`
}

type CloneDefinition struct {
	Lfs   string `yaml:"lfs,omitempty"`
	Depth int    `yaml:"depth,omitempty"`
}

type ServiceDefiniton struct {
	Image     string            `yaml:"image,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty"`
}

func (pd *PipelineStage) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var step SinglePipelineDefinition

	if err := unmarshal(&step); err == nil {
		pd.Steps = append(pd.Steps, step.SingleStep)
		return nil
	}

	var steps ParallelPipelineDefinition

	if err := unmarshal(&steps); err == nil {
		for _, step := range steps.ParallelSteps {
			pd.Steps = append(pd.Steps, step.SingleStep)
		}

		return nil
	}

	return unmarshal(&steps)
}
