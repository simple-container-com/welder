package pipelines

import (
	"github.com/smecsia/welder/pkg/pipelines/schema"
	"strings"
)

type BitbucketPipelinesRunParams struct {
	StepName      string // name of the step to run
	SkipPipes     bool   // if true, skip all pipes
	AtlassianMode bool   // if true, skip all Atlassian-specific steps and run extra steps
}

var (
	atlassianScriptsToSkip = map[string]bool{
		"source .artifactory/activate.sh": true, // artifactory addition
	}
	atlassianPipesToSkip = map[string]bool{
		"atlassian/artifactory-sidekick:v1": true, // artifactory
	}
)

func (p *BitbucketPipelinesRunParams) NormalizeDockerReference(reference string) string {
	if strings.HasPrefix(reference, "docker-proxy.services.atlassian.com") {
		return strings.ReplaceAll(reference, "docker-proxy.services.atlassian.com", "docker.simple-container.com")
	}
	return reference
}

func (p *BitbucketPipelinesRunParams) ShouldSkipScript(scripts schema.Script, index int) bool {
	if scripts.IsScript(index) {
		script, err := scripts.GetScript(index)
		if err == nil && p.shouldSkipScript(script) {
			return true
		}
	}
	if scripts.IsPipe(index) {
		if p.SkipPipes {
			return true
		}
		pipe, err := scripts.GetPipe(index)
		if err == nil && p.shouldSkipPipe(pipe) {
			return true
		}
	}
	return false
}

func (p *BitbucketPipelinesRunParams) shouldSkipPipe(pipe schema.Pipe) bool {
	if p.AtlassianMode {
		if atlassianPipesToSkip[strings.TrimSpace(pipe.Pipe)] {
			return true
		}
	}
	return false
}

func (p *BitbucketPipelinesRunParams) shouldSkipScript(script string) bool {
	if p.AtlassianMode {
		if atlassianScriptsToSkip[strings.TrimSpace(script)] {
			return true
		}
	}
	return false
}

func (p *BitbucketPipelinesRunParams) filterSteps(steps schema.StepsOrParallel) schema.StepsOrParallel {
	var filteredSteps schema.StepsOrParallel
	for idx := range steps {
		if steps.IsParallel(idx) {
			parallel, _ := steps.ToParallel(idx)
			for _, step := range parallel.Parallel {
				if p.StepName == "" ||
					(step.Step.Name != nil && *step.Step.Name == p.StepName) {
					filteredSteps = append(filteredSteps, step)
				}
			}
		}
		if steps.IsStep(idx) {
			step, _ := steps.ToStep(idx)
			if p.StepName == "" ||
				(step.Step.Name != nil && *step.Step.Name == p.StepName) {
				filteredSteps = append(filteredSteps, step)
			}
		}
	}
	return filteredSteps
}
