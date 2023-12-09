package docker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/docker/dockerext"
	"github.com/simple-container-com/welder/pkg/util"
)

// important to run on V2 build engine to enable buildkit extensions
const DefaultBuilderVersion = types.BuilderV1

const LabelNameBuildConfigHash = "AtlasBuildBuildConfigHash"

// NewDockerfile creates new Dockerfile instance with the default client
func NewDockerfile(ctx context.Context, filePath string, tags ...string) (*Dockerfile, error) {
	cliExt, err := dockerext.NewCLIExt(ctx)
	if err != nil {
		return nil, err
	}
	return &Dockerfile{
		FilePath:   filePath,
		Tags:       tags,
		client:     cliExt,
		TagDigests: make(map[string]TagDigest),
		rwMutex:    &sync.RWMutex{},
	}, nil
}

// Validates Dockerfile
// Limitations: very simple validation atm
func (dockerFile *Dockerfile) IsValid() (bool, error) {
	bytes, err := ioutil.ReadFile(dockerFile.FilePath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read Dockerfile from %s", dockerFile.FilePath)
	}
	return strings.Contains(string(bytes), "FROM "), nil
}

// Build builds Docker image from the specified Dockerfile with the specified tags
func (dockerFile *Dockerfile) Build() (MsgReader, error) {
	configHash, err := dockerFile.calcConfigHash()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to calc hash")
	}

	msgChan := make(chan readerNextMessage)
	msgReader := chanMsgReader{msgChan: msgChan, expectedEOFs: 1}

	// if reuse images with same cfg is allowed
	labels := util.CopyStringMap(dockerFile.Labels)
	if dockerFile.ReuseImagesWithSameCfg {
		images, err := dockerFile.client.API().ImageList(dockerFile.Context, types.ImageListOptions{All: true})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list images")
		}
		for _, image := range images {
			if image.Labels[LabelNameBuildConfigHash] == configHash {
				go func() {
					msgChan <- readerNextMessage{Message: ResponseMessage{
						Status: "Reusing Docker image", Id: image.ID,
					}}
					msgChan <- readerNextMessage{EOF: true}
				}()
				return &msgReader, nil
			}
		}
	}

	if !dockerFile.SkipHashLabel {
		labels[LabelNameBuildConfigHash] = configHash
	}
	dockerFilePath := filepath.Base(dockerFile.FilePath)
	contextPath := filepath.Dir(dockerFile.FilePath)
	if dockerFile.ContextPath != "" {
		if filepath.IsAbs(dockerFile.FilePath) {
			if relPath, err := filepath.Rel(dockerFile.ContextPath, dockerFile.FilePath); err != nil {
				return nil, errors.Wrapf(err, "specified Dockerfile '%s' is not within context path '%s'", dockerFile.FilePath, dockerFile.ContextPath)
			} else {
				dockerFilePath = relPath
			}
		} else {
			dockerFilePath = dockerFile.FilePath
		}
		contextPath = dockerFile.ContextPath
	}

	authConfigs := make(map[string]types.AuthConfig)
	if from, err := ParseFrom(dockerFile.FilePath); err != nil {
		return nil, err
	} else {
		registry, err := RegistryFromImageReference(from)
		if err != nil {
			return nil, err
		}
		authConfigs[from] = registry.AuthConfig
	}

	for _, tag := range dockerFile.Tags {
		registry, err := RegistryFromImageReference(tag)
		if err != nil {
			return nil, err
		}
		authConfigs[tag] = registry.AuthConfig
	}

	buildOptions := types.ImageBuildOptions{
		SuppressOutput: false,
		PullParent:     !dockerFile.DisablePull,
		Tags:           dockerFile.Tags,
		NoCache:        !dockerFile.DisableNoCache,
		Labels:         labels,
		Dockerfile:     dockerFilePath,
		BuildArgs:      dockerFile.Args,
		AuthConfigs:    authConfigs,
		Remove:         true,
		ForceRemove:    true,
		Version:        dockerFile.builderVersion(),
	}

	tarOptions, err := dockerFile.client.GetTarWithOptions(contextPath, dockerFilePath)
	if err != nil {
		return nil, err
	}
	resp, err := dockerFile.client.API().ImageBuild(dockerFile.GoContext(), tarOptions, buildOptions)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(resp.Body)

	go dockerFile.streamMessagesToChannel(reader, msgChan, "")

	return &msgReader, nil
}

// Push pushes Docker image to the registry defined by its tag
func (dockerFile *Dockerfile) Push() (MsgReader, error) {
	if len(dockerFile.Tags) == 0 {
		return nil, errors.New("no tags provided, hence could not push image")
	}

	msgChan := make(chan readerNextMessage)
	msgReader := chanMsgReader{msgChan: msgChan, expectedEOFs: len(dockerFile.Tags)}

	for _, tag := range dockerFile.Tags {
		registry, err := RegistryFromImageReference(tag)
		if err != nil {
			return nil, err
		}
		pushOpts := types.ImagePushOptions{
			RegistryAuth: registry.AuthHeader,
		}
		resp, err := dockerFile.client.API().ImagePush(dockerFile.GoContext(), tag, pushOpts)
		if err != nil {
			return nil, err
		}
		go dockerFile.streamMessagesToChannel(bufio.NewReader(resp), msgChan, tag)
	}
	return &msgReader, nil
}

func (dockerFile *Dockerfile) streamMessagesToChannel(reader *bufio.Reader, msgChan chan readerNextMessage, fullTag string) {
	scanner := util.NewLineOrReturnScanner(reader)
	for {
		if !scanner.Scan() {
			msgChan <- readerNextMessage{EOF: true}
			break
		}
		line := string(scanner.Bytes())
		err := scanner.Err()
		if err != nil {
			msgChan <- readerNextMessage{Error: err}
		}
		if buildKitMsg, err := dockerFile.tryReadBuildkitTraceResponse(line); err != nil {
			msgChan <- readerNextMessage{Error: err}
			continue
		} else if len(buildKitMsg) > 0 {
			for _, msg := range buildKitMsg {
				dockerFile.processDockerResponseMsg(msg, fullTag, msgChan)
			}
			continue // skip to next message
		} else {
			msg := ResponseMessage{}
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				msgChan <- readerNextMessage{Error: err}
			} else {
				dockerFile.processDockerResponseMsg(msg, fullTag, msgChan)
			}
		}
	}
}

func (dockerFile *Dockerfile) processDockerResponseMsg(msg ResponseMessage, fullTag string, msgChan chan readerNextMessage) {
	if msg.Aux.ID != "" {
		dockerFile.id = msg.Aux.ID
	} else if msg.Aux.Digest != "" && msg.Aux.Tag != "" {
		tagDigest := TagDigest{
			Tag:    msg.Aux.Tag,
			Digest: msg.Aux.Digest,
			Size:   msg.Aux.Size,
		}
		dockerFile.rwMutex.Lock()
		dockerFile.TagDigests[fullTag] = tagDigest
		dockerFile.rwMutex.Unlock()
	} else if msg.Error != "" {
		msgChan <- readerNextMessage{Error: errors.New(msg.Error)}
	} else {
		msgChan <- readerNextMessage{Message: msg}
	}
}

func ParseFrom(dockerFilePath string) (string, error) {
	dockerFileBytes, err := ioutil.ReadFile(dockerFilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read Dockerfile from %s", dockerFilePath)
	}
	refRegex := reference.NameRegexp.String() +
		"(?::" + reference.TagRegexp.String() + ")?" +
		"(?:@" + reference.DigestRegexp.String() + ")?"
	dockerFileRegexp := "(?i)FROM (" + refRegex + ")"
	fromRegexp := regexp.MustCompile(dockerFileRegexp)
	if !fromRegexp.Match(dockerFileBytes) {
		return "", errors.Errorf("could not parse provided Dockerfile from %s", dockerFilePath)
	}
	dockerFileContents := string(dockerFileBytes)
	fromValue := fromRegexp.FindStringSubmatch(dockerFileContents)
	return fromValue[1], nil
}

// calcConfigHash calculates hash sum of configuration (to figure out whether container needs to be re-created)
func (dockerFile *Dockerfile) calcConfigHash() (string, error) {
	var b bytes.Buffer
	gob.Register(dockerFile.Args)
	gob.Register(dockerFile.Labels)
	content, err := ioutil.ReadFile(dockerFile.FilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read content of Dockerfile: "+dockerFile.FilePath)
	}
	err = gob.NewEncoder(&b).Encode([]interface{}{
		content, dockerFile.ContextPath,
		dockerFile.Args, dockerFile.Tags, dockerFile.Labels,
	})
	hash := md5.New()
	hash.Write(b.Bytes())
	return hex.EncodeToString(hash.Sum(nil)), err
}

func (dockerFile *Dockerfile) builderVersion() types.BuilderVersion {
	if dockerFile.BuilderVersion == "" {
		return DefaultBuilderVersion
	}
	return types.BuilderVersion(dockerFile.BuilderVersion)
}

func (dockerFile *Dockerfile) tryReadBuildkitTraceResponse(line string) ([]ResponseMessage, error) {
	var res []ResponseMessage
	if dockerFile.builderVersion() == types.BuilderBuildKit {
		buildRes := types.BuildResult{}
		if err := json.Unmarshal([]byte(line), &buildRes); err != nil {
			return res, errors.Wrapf(err, "failed to unmarshal line: %s", line)
		}
		switch buildRes.ID {
		case "moby.buildkit.trace":
			msg2 := jsonmessage.JSONMessage{}
			var resp controlapi.StatusResponse
			if err := json.Unmarshal([]byte(line), &msg2); err != nil {
				return res, errors.Wrapf(err, "failed to unmarshal buildkit.trace line: %s", line)
			}
			var dt []byte
			// ignoring all messages that are not understood
			if err := json.Unmarshal(*msg2.Aux, &dt); err != nil {
				return res, errors.Wrapf(err, "failed to unmarshal aux: %s", *msg2.Aux)
			}
			if err := (&resp).Unmarshal(dt); err != nil {
				return res, errors.Wrapf(err, "failed to unmarshal PB message: %s", dt)
			}
			res = append(res, ResponseMessage{
				Id:     msg2.ID,
				Status: resp.String(),
			})
			return res, nil
		}
	}
	return res, nil
}
