/*
*
This entire package is a copy of Docker sources to publicize and use the ability to
run exec from within the command line seamlessly
*/
package dockerext

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/image/build"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/moby/term"
	"github.com/pkg/errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type DockerCLIExt interface {
	API() client.APIClient
	GetTarWithOptions(contextPath string, dockerFilePath string) (io.Reader, error)
	Ping() error
}

type dockerCLIExt struct {
	client  client.APIClient
	stout   *streams.Out
	stin    *streams.In
	sterr   io.Writer
	context context.Context
}

func NewCLIExt(ctx context.Context) (DockerCLIExt, error) {
	stdin, stdout, stderr := term.StdStreams()

	configFile := cliconfig.LoadDefaultConfigFile(stderr)
	storeConfig := command.DefaultContextStoreConfig()

	baseContextStore := store.New(cliconfig.ContextStoreDir(), storeConfig)
	contextStore := &command.ContextStoreWithDefault{
		Store: baseContextStore,
		Resolver: func() (*command.DefaultContext, error) {
			return command.ResolveDefaultContext(flags.NewCommonOptions(), configFile, storeConfig, stderr)
		},
	}
	dockerEndpoint, err := resolveDockerEndpoint(contextStore, command.DefaultContextName)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve docker endpoint")
	}
	dockerAPI, err := newAPIClientFromEndpoint(dockerEndpoint, configFile)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve docker endpoint")
	}

	cliExt := &dockerCLIExt{
		client:  dockerAPI,
		stout:   streams.NewOut(stdout),
		sterr:   stderr,
		stin:    streams.NewIn(stdin),
		context: ctx,
	}
	return cliExt, cliExt.Ping()
}

func (cli *dockerCLIExt) API() client.APIClient {
	return cli.client
}

func (cli *dockerCLIExt) Ping() error {
	ping, err := cli.client.Ping(cli.context)
	if err != nil {
		return err
	}
	if ping.APIVersion != "" {
		cli.client.NegotiateAPIVersionPing(ping)
	}
	return nil
}

func (cli *dockerCLIExt) GetTarWithOptions(contextPath string, dockerFilePath string) (io.Reader, error) {
	contextDir := filepath.Dir(dockerFilePath)
	options := archive.TarOptions{
		ChownOpts: &idtools.Identity{UID: 0, GID: 0},
	}
	if contextPath != "" {
		contextDir = contextPath
	}
	ignoreFile := filepath.Join(contextPath, ".dockerignore")
	if _, err := os.Stat(ignoreFile); err == nil {
		dockerignoreFile, err := os.Open(ignoreFile)
		if err != nil {
			fmt.Println("failed to read .dockerignore file: " + err.Error())
		} else if patterns, err := dockerignore.ReadAll(dockerignoreFile); err != nil {
			fmt.Println("failed to read .dockerignore file: " + err.Error())
		} else {
			excludePatterns := make([]string, 0)
			for _, pattern := range patterns {
				if pattern != dockerFilePath {
					excludePatterns = append(excludePatterns, pattern)
				}
			}
			options.ExcludePatterns = excludePatterns
		}
	}
	if err := build.ValidateContextDirectory(contextDir, options.ExcludePatterns); err != nil {
		return nil, errors.Errorf("error checking context: '%s'.", err)
	}

	relDockerfile := archive.CanonicalTarNameForPath(dockerFilePath)

	options.ExcludePatterns = build.TrimBuildFilesFromExcludes(options.ExcludePatterns, relDockerfile, false)

	if ctx, err := archive.TarWithOptions(contextDir, &options); err != nil {
		return nil, errors.Wrapf(err, "failed to init tar with options")
	} else {
		// Setup an upload progress bar
		progressOutput := streamformatter.NewProgressOutput(cli.stout)
		if !cli.stout.IsTerminal() {
			progressOutput = &LastProgressOutput{Output: progressOutput}
		}
		return progress.NewProgressReader(ctx, progressOutput, 0, "", "Sending build context to Docker daemon"), nil
	}
}

func withHTTPClient(tlsConfig *tls.Config) func(*client.Client) error {
	return func(c *client.Client) error {
		if tlsConfig == nil {
			// Use the default HTTPClient
			return nil
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				DialContext: (&net.Dialer{
					KeepAlive: 30 * time.Second,
					Timeout:   30 * time.Second,
				}).DialContext,
			},
			CheckRedirect: client.CheckRedirect,
		}
		return client.WithHTTPClient(httpClient)(c)
	}
}

func resolveDockerEndpoint(s store.Reader, contextName string) (docker.Endpoint, error) {
	ctxMeta, err := s.GetMetadata(contextName)
	if err != nil {
		return docker.Endpoint{}, err
	}
	epMeta, err := docker.EndpointFromContext(ctxMeta)
	if err != nil {
		return docker.Endpoint{}, err
	}
	return docker.WithTLSData(s, contextName, epMeta)
}

func newAPIClientFromEndpoint(ep docker.Endpoint, configFile *configfile.ConfigFile) (client.APIClient, error) {
	clientOpts, err := ep.ClientOpts()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := tlsConfig(&ep)
	if err != nil {
		return nil, err
	}
	customHeaders := configFile.HTTPHeaders
	if customHeaders == nil {
		customHeaders = map[string]string{}
	}
	customHeaders["User-Agent"] = command.UserAgent()
	clientOpts = append(clientOpts, client.WithAPIVersionNegotiation())
	clientOpts = append(clientOpts, client.WithHTTPHeaders(customHeaders))
	clientOpts = append(clientOpts, withHTTPClient(tlsConfig))
	return client.NewClientWithOpts(clientOpts...)
}

// tlsConfig extracts a context docker endpoint TLS config
func tlsConfig(c *docker.Endpoint) (*tls.Config, error) {
	if c.TLSData == nil && !c.SkipTLSVerify {
		// there is no specific tls config
		return nil, nil
	}
	var tlsOpts []func(*tls.Config)
	if c.TLSData != nil && c.TLSData.CA != nil {
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(c.TLSData.CA) {
			return nil, errors.New("failed to retrieve context tls info: ca.pem seems invalid")
		}
		tlsOpts = append(tlsOpts, func(cfg *tls.Config) {
			cfg.RootCAs = certPool
		})
	}
	if c.TLSData != nil && c.TLSData.Key != nil && c.TLSData.Cert != nil {
		keyBytes := c.TLSData.Key
		pemBlock, _ := pem.Decode(keyBytes)
		if pemBlock == nil {
			return nil, fmt.Errorf("no valid private key found")
		}

		var err error
		if x509.IsEncryptedPEMBlock(pemBlock) {
			keyBytes, err = x509.DecryptPEMBlock(pemBlock, []byte(c.TLSPassword))
			if err != nil {
				return nil, errors.Wrap(err, "private key is encrypted, but could not decrypt it")
			}
			keyBytes = pem.EncodeToMemory(&pem.Block{Type: pemBlock.Type, Bytes: keyBytes})
		}

		x509cert, err := tls.X509KeyPair(c.TLSData.Cert, keyBytes)
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve context tls info")
		}
		tlsOpts = append(tlsOpts, func(cfg *tls.Config) {
			cfg.Certificates = []tls.Certificate{x509cert}
		})
	}
	if c.SkipTLSVerify {
		tlsOpts = append(tlsOpts, func(cfg *tls.Config) {
			cfg.InsecureSkipVerify = true
		})
	}
	return tlsconfig.ClientDefault(tlsOpts...), nil
}
