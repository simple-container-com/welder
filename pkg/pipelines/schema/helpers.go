package schema

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Parallel struct {
	Parallel []Step `json:"parallel,omitempty"`
}

func (sop StepsOrParallel) IsParallel(idx int) bool {
	if gMap, ok := sop[idx].(map[interface{}]interface{}); ok {
		_, isParallel := gMap["parallel"]
		return isParallel
	} else if sMap, ok := sop[idx].(map[string]interface{}); ok {
		_, isParallel := sMap["parallel"]
		return isParallel
	}
	return false
}

func (sop StepsOrParallel) IsStep(idx int) bool {
	if gMap, ok := sop[idx].(map[interface{}]interface{}); ok {
		_, isStep := gMap["step"]
		return isStep
	} else if sMap, ok := sop[idx].(map[string]interface{}); ok {
		_, isStep := sMap["step"]
		return isStep
	}
	return false
}

func (sop StepsOrParallel) ToStep(idx int) (Step, error) {
	res := Step{}
	value := sop[idx]
	bytes, err := yaml.Marshal(value)
	if err != nil {
		return res, errors.Wrapf(err, "failed to marshal %s", value)
	}
	if err := yaml.Unmarshal(bytes, &res); err != nil {
		return res, errors.Wrapf(err, "failed to unmarshal %s", value)
	}
	return res, nil
}

func (sop StepsOrParallel) ToParallel(idx int) (Parallel, error) {
	res := Parallel{}
	value := sop[idx]
	bytes, err := yaml.Marshal(value)
	if err != nil {
		return res, errors.Wrapf(err, "failed to marshal %s", value)
	}
	if err := yaml.Unmarshal(bytes, &res); err != nil {
		return res, errors.Wrapf(err, "failed to unmarshal %s", value)
	}
	return res, nil
}

func (np NamedPipelines) Get(name string) (StepsOrParallel, error) {
	res := StepsOrParallel{}
	namedPipeline, ok := np[name]
	if !ok {
		return res, errors.Errorf("failed to get named pipeline %q", name)
	}
	bytes, err := yaml.Marshal(namedPipeline)
	if err != nil {
		return res, errors.Wrapf(err, "failed to marshal %s", namedPipeline)
	}
	if err := yaml.Unmarshal(bytes, &res); err != nil {
		return res, errors.Wrapf(err, "failed to unmarshal %s", namedPipeline)
	}
	return res, nil
}

func (j *StepStep) GetScript(idx int) (string, error) {
	scriptIdx := j.Script[idx]
	stringVal, ok := scriptIdx.(string)
	if !ok {
		return "", errors.Errorf("failed to convert sript %d (%s) to string", idx, j.Script[idx])
	}
	return stringVal, nil
}

func (np NamedPipelines) Contains(name string) bool {
	_, ok := np[name]
	return ok
}

func (s Script) IsPipe(idx int) bool {
	_, ok := s[idx].(map[interface{}]interface{})
	return ok
}

func (s Script) IsScript(idx int) bool {
	_, ok := s[idx].(string)
	return ok
}

func (s Script) GetPipe(idx int) (Pipe, error) {
	res := Pipe{}
	pipe, ok := s[idx].(map[interface{}]interface{})
	if !ok {
		return res, errors.Errorf("failed to convert script (%s) to pipe", s[idx])
	}
	bytes, err := yaml.Marshal(pipe)
	if err != nil {
		return res, errors.Wrapf(err, "failed to marshal %s", pipe)
	}
	if err := yaml.Unmarshal(bytes, &res); err != nil {
		return res, errors.Wrapf(err, "failed to unmarshal %s", pipe)
	}
	return res, nil
}

func (s Script) GetScript(idx int) (string, error) {
	script, ok := s[idx].(string)
	if !ok {
		return "", errors.Errorf("failed to convert script (%s) to string", s[idx])
	}
	return script, nil
}

func (j *StepStep) GetImage(root BitbucketPipelinesSchemaJson) (string, error) {
	if j.Image != nil {
		return bbImageToDockerImage(j.Image)
	}
	return bbImageToDockerImage(root.Image)
}

func bbImageToDockerImage(imgObj interface{}) (string, error) {
	stringImage, ok := imgObj.(string)
	if ok {
		return stringImage, nil
	}
	// now it must be a complex image
	complexImage, ok := imgObj.(map[interface{}]interface{})
	if !ok {
		return "", errors.Errorf("cannot convert BBP image to Docker image: %s", complexImage)
	}
	if _, ok := complexImage["username"]; ok {
		// complex image with only name
		return complexImage["name"].(string), nil
	} else if name, ok := complexImage["name"]; ok {
		// complex image with only name
		return name.(string), nil
	}

	return "", errors.Errorf("failed to convert image to Docker image %s", imgObj)
}

func (v *PipeVariables) ToEnv() []string {
	res := make([]string, len(*v))
	var i int
	for k, v := range *v {
		res[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	return res
}

func (j *BitbucketPipelinesSchemaJson) AllSteps() []Step {
	var res []Step
	res = appendStepsFromNamedPipelineSafe(j.Pipelines.Branches, res)
	res = appendStepsFromNamedPipelineSafe(j.Pipelines.Tags, res)
	res = appendStepsFromNamedPipelineSafe(j.Pipelines.PullRequests, res)
	res = appendStepsSafe(j.Pipelines.Default, res)
	return res
}

func appendStepsFromNamedPipelineSafe(pipeline NamedPipelines, steps []Step) []Step {
	for name := range pipeline {
		stepsToAdd, err := pipeline.Get(name)
		if err == nil {
			steps = appendStepsSafe(stepsToAdd, steps)
		}
	}
	return steps
}

func appendStepsSafe(stepsToAdd StepsOrParallel, steps []Step) []Step {
	for idx := range stepsToAdd {
		if stepsToAdd.IsStep(idx) {
			if step, err := stepsToAdd.ToStep(idx); err == nil {
				steps = append(steps, step)
			}
		} else if stepsToAdd.IsParallel(idx) {
			if parallel, err := stepsToAdd.ToParallel(idx); err == nil {
				steps = append(steps, parallel.Parallel...)
			}
		}
	}
	return steps
}
