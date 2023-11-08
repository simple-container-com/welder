package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

type Image struct {
	Reference string
	Name      string
	Digest    string
}

type Registry struct {
	AuthHeader string
	AuthValue  string
	AuthConfig types.AuthConfig
}

const DefaultDockerRegistry = "index.docker.io"

var urlRegexp = regexp.MustCompile(`https?://.+`)

// ResolveDockerImageReference resolves valid docker image reference
// reference can be represented in the format <image-name>@<digest> or <image-name>:<tag>
func ResolveDockerImageReference(reference string) (Image, error) {
	ref, err := name.ParseReference(reference, name.WeakValidation)
	if err != nil {
		return Image{}, fmt.Errorf("parsing reference %q: %v", reference, err)
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return Image{}, fmt.Errorf("reading image %q: %v", ref, err)
	}
	hash, err := img.Digest()
	if err != nil {
		return Image{}, fmt.Errorf("gettig image hash %q: %v", ref, err)
	}
	return Image{
		Reference: fmt.Sprintf("%s@%s", ref.Context(), hash),
		Name:      ref.Context().String(),
		Digest:    hash.String(),
	}, nil
}

// TagFromReference returns image tag from full reference
func TagFromReference(reference string) (string, error) {
	ref, err := name.NewTag(reference, name.WeakValidation)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %v", reference, err)
	}
	return ref.Identifier(), nil
}

// ImageFromReference returns image name from full reference
func ImageFromReference(reference string) (string, error) {
	ref, err := name.ParseReference(reference, name.WeakValidation)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %v", reference, err)
	}
	return ref.Context().String(), nil
}

// ImageAndTagFromFullReference returns image and tag from full reference
func ImageAndTagFromFullReference(ref string) (string, string, error) {
	image, err := ImageFromReference(ref)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine image name from tag: %s", ref)
	}
	tag, err := TagFromReference(ref)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine image tag: %s", tag)
	}
	return image, tag, nil
}

// RegistryFromImageReference allows to figure out the Docker registry context, such as authentication
func RegistryFromImageReference(reference string) (*Registry, error) {
	ref, err := name.ParseReference(reference, name.WeakValidation)
	if err != nil {
		return nil, fmt.Errorf("parsing reference %q: %v", reference, err)
	}
	return ResolveRegistry(ref.Context().RegistryStr())
}

// ResolveRegistry resolves registry by its name (URL) and provides auth header
// this can be useful when working with Docker registry through Docker CLI (daemon and SDK)
func ResolveRegistry(registryName string) (*Registry, error) {
	registry, err := name.NewRegistry(registryName, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init registry object for %s", registryName)
	}
	creds, err := authn.DefaultKeychain.Resolve(registry)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get valid creds for registry %q", registryName)
	}
	authObj, err := creds.Authorization()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get auth from creds for registry %q", registryName)
	}
	authConfig := types.AuthConfig{}
	if authObj.Username == "" && authObj.RegistryToken == "" { // fallback to Docker CLI API
		cli, err := command.NewDockerCli()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create new docker cli")
		}
		err = cli.Initialize(cliflags.NewClientOptions())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to init docker cli")
		}
		store := cli.ConfigFile().GetCredentialsStore(registryName)
		storeEntryName := registryName
		if registryName == DefaultDockerRegistry {
			storeEntryName = fmt.Sprintf("https://%s/v1/", DefaultDockerRegistry)
		}
		cliAuthObj, err := store.Get(storeEntryName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read auth config")
		}
		authConfig.RegistryToken = cliAuthObj.RegistryToken
		authConfig.IdentityToken = cliAuthObj.IdentityToken
		authConfig.Auth = cliAuthObj.Auth
		authConfig.Username = cliAuthObj.Username
		authConfig.Password = cliAuthObj.Password
		authConfig.ServerAddress = cliAuthObj.ServerAddress
	} else {
		authConfig.RegistryToken = authObj.RegistryToken
		authConfig.IdentityToken = authObj.IdentityToken
		authConfig.Auth = authObj.Auth
		authConfig.Username = authObj.Username
		authConfig.Password = authObj.Password
	}
	authValue := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", authObj.Username, authObj.Password)))
	if token, err := encodeDockerAuthHeader(authConfig); err != nil {
		return nil, err
	} else {
		return &Registry{AuthHeader: token, AuthValue: authValue, AuthConfig: authConfig}, nil
	}
}

// ReadDockerConfigJson  reads ~/.docker/config.json file at a default location
func ReadDockerConfigJson() (HomeDockerConfig, error) {
	res := HomeDockerConfig{}
	configJsonPath, err := homedir.Expand("~/.docker/config.json")
	if err != nil {
		return res, errors.Wrapf(err, "failed to resolve docker config.json")
	}
	_, err = os.Stat(configJsonPath)
	if os.IsNotExist(err) {
		return res, nil
	}
	configJsonBytes, err := ioutil.ReadFile(configJsonPath)
	if err != nil {
		return res, errors.Wrapf(err, "failed to read ~/.docker/config.json")
	}
	err = json.Unmarshal(configJsonBytes, &res)
	if err != nil {
		return res, errors.Wrapf(err, "failed to unmarshall ~/.docker/config.json")
	}
	return res, nil
}

// ResolveExternalAuths resolves authentication details for each registry listed in config.json
func (cfg *HomeDockerConfig) ResolveExternalAuths() error {
	for externalRegistry := range cfg.Auths {
		if strings.Contains(externalRegistry, DefaultDockerRegistry) {
			externalRegistry = DefaultDockerRegistry
		}
		if urlRegexp.Match([]byte(externalRegistry)) {
			regUrl, _ := url.Parse(externalRegistry)
			regUrlPath := ""
			if regUrl.Path != "" {
				regUrlPath = "/" + regUrl.Path
			}
			externalRegistry = fmt.Sprintf("%s:%s%s", regUrl.Host, regUrl.Port(), regUrlPath)
		}
		reg, err := ResolveRegistry(externalRegistry)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve registry %s", externalRegistry)
		}
		cfg.Auths[externalRegistry] = HomeDockerConfigAuth{
			Auth: reg.AuthValue,
		}
	}
	return nil
}

// DumpToTmpFile creates temporary .docker/config.json file with registry authentication
func (cfg *HomeDockerConfig) DumpToTmpFile() (string, error) {
	contBytes, err := json.Marshal(cfg)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temp file")
	}
	systemTmp, err := ioutil.TempDir("", ".docker")
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temporary dir")
	}
	tmpFile := path.Join(systemTmp, "config.json")
	if err := ioutil.WriteFile(tmpFile, contBytes, os.ModePerm); err != nil {
		return "", errors.Wrapf(err, "failed to write temporary config.json")
	}
	return tmpFile, nil
}

func encodeDockerAuthHeader(authConfig types.AuthConfig) (string, error) {
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}
