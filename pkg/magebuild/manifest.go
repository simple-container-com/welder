package magebuild

import (
	"context"
	"fmt"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"
)

const (
	errCreate   = "Failed to create file at %q"
	errWrite    = "Failed to write file at %q"
	errDownload = "Failed to to download file from %q, HTTP status: %d"
)

type Release struct {
	Channels  []string  `toml:"channels,omitempty"`
	Timestamp time.Time `toml:"timestamp,omitempty"`
	Variants  []string  `toml:"variants,omitempty"`
	Version   string    `toml:"version,omitempty"`
}

type SLAuthToken struct {
	Audiences []string `toml:"audiences,omitempty"`
	Groups    []string `toml:"groups,omitempty"`
	Mfa       bool     `toml:"mfa,omitempty"`
}

// PluginManifest represents atlas-cli plugin manifest
// The original structure (stash.atlassian.com/micros/atlas-cli/pkg/config/plugin_manifest.go) is not
// compatible with github.com/pelletier/go-toml
// and github.com/BurntSushi/toml does not allow marshalling
type PluginManifest struct {
	Owners      []string    `toml:"owners,omitempty"`
	Description string      `toml:"description,omitempty"`
	Tags        []string    `toml:"tags,omitempty"`
	Summary     string      `toml:"summary,omitempty"`
	Name        string      `toml:"name,omitempty"`
	Releases    []Release   `toml:"release,omitempty"`
	SLAuthToken SLAuthToken `toml:"slauthtoken,omitempty"`
}

// ReadCurrentPluginManifest reads current plugin manifest from target
func (ctx *GoBuildContext) ReadCurrentPluginManifest(target Target) (*PluginManifest, error) {
	return readManifestFile(ctx.ManifestFile(target))
}

// GetLatestPluginManifest downloads latest plugin manifest from the given url
func (ctx *GoBuildContext) GetLatestPluginManifest(target Target) (*PluginManifest, error) {
	tmpFile, err := ioutil.TempFile("", target.Name+"-manifest.toml")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create temporary file")
	}

	repository := ctx.BaseTargetURL(target)
	u, err := url.Parse(repository)
	if err != nil {
		return nil, errors.Wrapf(err, "Invalid repository URL: %s", repository)
	}
	u.Path = path.Join(u.Path, "manifest.toml")

	res, err := get(context.TODO(), u.String())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to download")
	}
	if err := copyToFile(res, tmpFile.Name()); err != nil {
		if err == NotFoundError {
			return nil, err
		}
		return nil, errors.Wrapf(err, "failed to download latest manifest file")
	}

	return readManifestFile(tmpFile.Name())
}

// Get allows a download to be canceled early.
func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create http request")
	}
	req = req.WithContext(ctx)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to invoke http request")
	}

	return res, nil
}

var NotFoundError = errors.New("not found")

// copyToFile creates a file and reads a response body into it.
func copyToFile(res *http.Response, file string) error {
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return NotFoundError
	}

	if res.StatusCode != http.StatusOK {
		return errors.Errorf(errDownload, res.Request.URL.String(), res.StatusCode)
	}

	dir := filepath.Dir(file)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return errors.Wrapf(err, "Failed to create directory %q", dir)
	}

	out, err := os.Create(file)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf(errCreate, file))
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, res.Body)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf(errWrite, file))
	}

	return nil
}

func readManifestFile(file string) (*PluginManifest, error) {
	latestPMBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read TOML file")
	}
	latestPM := PluginManifest{}
	if err := toml.Unmarshal(latestPMBytes, &latestPM); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal TOML")
	}
	return &latestPM, nil
}

// PublishPluginManifest uploads manifest of a target to statlas
func (ctx *GoBuildContext) PublishPluginManifest(target Target, manifest *PluginManifest) error {
	fmt.Println("Publishing manifest with ", len(manifest.Releases), " releases...")
	manifestFile, err := ioutil.TempFile("", target.Name+"-manifest.toml")
	if err != nil {
		return errors.Wrapf(err, "failed to create temporary file")
	}
	tomlBytes, err := toml.Marshal(*manifest)
	fmt.Println("Here it goes: \n", string(tomlBytes))
	if err != nil {
		return errors.Wrapf(err, "failed to marshal manifest to TOML")
	}
	if err := ioutil.WriteFile(manifestFile.Name(), tomlBytes, os.ModePerm); err != nil {
		return errors.Wrapf(err, "failed to write temporary manifest file")
	}
	checksumFile := fmt.Sprintf("%s.sha256", manifestFile.Name())
	if err := ctx.WriteFileChecksum(manifestFile.Name(), checksumFile); err != nil {
		return errors.Wrapf(err, "failed to write temporary manifest checksum file")
	}

	latestPMBytes, _ := ioutil.ReadFile(manifestFile.Name())
	fmt.Println(string(latestPMBytes))
	return ctx.UploadFileToStatlas(ctx.BaseTargetURL(target), "manifest.toml", manifestFile.Name(), checksumFile)

}

// InitManifest uploads manifest file into statlas
func (ctx *GoBuildContext) InitManifest(target Target) error {
	currentManifest, err := ctx.ReadCurrentPluginManifest(target)
	if err != nil {
		return err
	}
	return ctx.PublishPluginManifest(target, currentManifest)
}

// PublishNewRelease downloads info about releases from statlas and adds new release into it
func (ctx *GoBuildContext) PublishNewRelease(target Target, version string) error {
	currentManifest, err := ctx.ReadCurrentPluginManifest(target)
	if err != nil {
		return err
	}
	latestManifest, err := ctx.GetLatestPluginManifest(target)
	if err == NotFoundError {
		latestManifest = currentManifest
	} else if err != nil {
		return err
	}
	currentManifest.Releases = latestManifest.Releases
	newRelease := true
	for _, release := range currentManifest.Releases {
		if release.Version == version {
			newRelease = false
			break
		}
	}
	if newRelease {
		currentManifest.Releases = append([]Release{{
			Channels:  []string{ctx.ReleaseChannel},
			Version:   version,
			Timestamp: time.Now(),
			Variants:  ctx.PlatformsToVariants(),
		}}, currentManifest.Releases...)
	}
	return ctx.PublishPluginManifest(target, currentManifest)
}
