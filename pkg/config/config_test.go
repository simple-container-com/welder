package config

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/util/test"
)

// Platform defines platform to run build for
type Platform struct {
	GOOS   string `yaml:"os,omitempty"`
	GOARCH string `yaml:"arch,omitempty"`
}

// Target defines target to run build for
type Target struct {
	Name string `yaml:"name,omitempty"`
	Path string `yaml:"path,omitempty"`
}

type BaseConfig struct {
	OutDir            string  `yaml:"outDir,omitempty" env:"OUT_DIR" default:"bin"`
	Version           string  `yaml:"version,omitempty" env:"VVERSION" default:""`
	StringWithDefault string  `yaml:"stringWithDefault,omitempty" default:"someDefault"`
	IntWithDefault    int     `yaml:"intWithDefault,omitempty" default:"32"`
	FloatWithDefault  float64 `yaml:"floatWithDefault,omitempty" default:"64.64"`
	BoolWithDefault   bool    `yaml:"boolWithDefault,omitempty" default:"true"`
	Timestamp         int     `yaml:"timestamp,omitempty" env:"TIMESTAMP" default:"1"`

	// default-only fields
	configFilePath string
	initialized    bool
}

// Context build context and config
type TestConfig struct {
	BaseConfig   `yaml:",inline"`
	ArmoryURL    string     `yaml:"armoryURL,omitempty" env:"ARMORY_URL" default:"https://armory.prod.simple-container.com"`
	TrebuchetURL string     `yaml:"trebuchetURL,omitempty" env:"TREBUCHET_URL" default:"https://trebuchet.prod.simple-container.com"`
	Platforms    []Platform `yaml:"platforms,omitempty"`
	Targets      []Target   `yaml:"targets,omitempty"`

	// env-only fields
	IsParallel  bool   `yaml:"-" default:"true" env:"PARALLEL"`
	IsSkipTests string `yaml:"-" default:"false" env:"SKIP_TESTS"`
}

func (tc *BaseConfig) SetConfigFilePath(path string) {
	tc.configFilePath = path
}

func (tc *BaseConfig) GetConfigFilePath() string {
	return tc.configFilePath
}

func (tc *BaseConfig) Init() error {
	tc.initialized = true
	return nil
}

func TestNewConfig(t *testing.T) {
	RegisterTestingT(t)

	defaultConfig := DefaultConfig(&TestConfig{}).(*TestConfig)
	result := AddDefaults(map[string]interface{}{"armoryURL": "", "outDir": ""}, &TestConfig{ArmoryURL: "http://blabla", BaseConfig: BaseConfig{OutDir: "bla"}}).(*TestConfig)
	Expect(result.initialized).To(Equal(false))
	Expect(result.ArmoryURL).To(Equal("http://blabla"))
	Expect(result.OutDir).To(Equal("bla"))
	Expect(result.TrebuchetURL).To(Equal(defaultConfig.TrebuchetURL))
}

func TestProvidedDefault(t *testing.T) {
	RegisterTestingT(t)

	config := AddEnv(DefaultConfig(&BaseConfig{
		StringWithDefault: "OverridenDefault",
		IntWithDefault:    128,
		FloatWithDefault:  128.128,
		BoolWithDefault:   false,
	})).(*BaseConfig)
	Expect(config.StringWithDefault).To(Equal("OverridenDefault"))
	Expect(config.IntWithDefault).To(Equal(128))
	Expect(config.FloatWithDefault).To(Equal(128.128))
	Expect(config.BoolWithDefault).To(Equal(true)) // false-values are non-overridable

	config = AddEnv(DefaultConfig(&BaseConfig{})).(*BaseConfig)
	Expect(config.StringWithDefault).To(Equal("someDefault"))
	Expect(config.IntWithDefault).To(Equal(32))
	Expect(config.FloatWithDefault).To(Equal(64.64))
	Expect(config.BoolWithDefault).To(Equal(true))
}

func TestNewConfigWithEnvironment(t *testing.T) {
	RegisterTestingT(t)

	defer os.Setenv("OUT_DIR", "")
	defer os.Setenv("ARMORY_URL", "")
	defer os.Setenv("PARALLEL", "")
	defer os.Setenv("TIMESTAMP", "")
	os.Setenv("ARMORY_URL", "http://blablabla")
	os.Setenv("OUT_DIR", "somedir")
	os.Setenv("PARALLEL", "false")
	os.Setenv("TIMESTAMP", "10")

	config := AddEnv(DefaultConfig(&TestConfig{})).(*TestConfig)
	Expect(config.initialized).To(Equal(false))
	Expect(config.ArmoryURL).To(Equal("http://blablabla"))
	Expect(config.OutDir).To(Equal("somedir"))
	Expect(config.IsParallel).To(Equal(false))
	Expect(config.Timestamp).To(Equal(10))
}

func TestReadConfig(t *testing.T) {
	RegisterTestingT(t)
	mockedReader := new(test.ConsoleReaderMock)
	defaultConfig := DefaultConfig(&TestConfig{}).(*TestConfig)
	mockedReader.On("ReadLine").Return("1.0.0", nil)
	defer os.Setenv("ARMORY_URL", "")
	defer os.Setenv("SKIP_TESTS", "")
	os.Setenv("ARMORY_URL", "http://blablabla")
	os.Setenv("SKIP_TESTS", "true")

	config := ReadConfig(AddEnv(DefaultConfig(&TestConfig{})).(Config), mockedReader).(*TestConfig)
	Expect(config.initialized).To(Equal(false))
	Expect(config.ArmoryURL).To(Equal("http://blablabla"))
	Expect(config.Version).To(Equal("1.0.0"))
	Expect(config.TrebuchetURL).To(Equal(defaultConfig.TrebuchetURL))
	Expect(config.IsParallel).To(Equal(true))
	Expect(config.IsSkipTests).To(Equal("true"))
}

func TestItShouldNotReadVersionIfSetInEnvVar(t *testing.T) {
	RegisterTestingT(t)
	mockedReader := new(test.ConsoleReaderMock)
	mockedReader.On("ReadLine").Return("SomeString", nil)
	defer os.Setenv("VVERSION", "")
	os.Setenv("VVERSION", "1.0.0")

	config := ReadConfig(AddEnv(DefaultConfig(&TestConfig{})).(Config), mockedReader).(*TestConfig)

	Expect(config.Version).To(Equal("1.0.0"))
	mockedReader.AssertNotCalled(t, "ReadLine")
}

func TestReadConfigFile(t *testing.T) {
	RegisterTestingT(t)

	readConfig, rawConfig, err := ReadConfigFile("testdata/build.yaml", AddEnv(DefaultConfig(&TestConfig{})).(Config))
	Expect(err).To(BeNil())

	config := AddDefaults(rawConfig, readConfig).(*TestConfig)

	defaultConfig := DefaultConfig(&TestConfig{}).(*TestConfig)
	Expect(config.initialized).To(Equal(false))
	Expect(config.ArmoryURL).To(Equal("http://armory.local"))
	Expect(config.TrebuchetURL).To(Equal(defaultConfig.TrebuchetURL))
	Expect(config.Platforms).To(HaveLen(2))
	Expect(config.Targets).To(HaveLen(1))
	Expect(config.IsParallel).To(Equal(true))
	Expect(config.GetConfigFilePath()).To(Equal("testdata/build.yaml"))
	Expect(config.IsSkipTests).To(Equal("false"))
}

func TestInit(t *testing.T) {
	RegisterTestingT(t)

	defer os.Setenv("SKIP_TESTS", "")
	os.Setenv("SKIP_TESTS", "true")
	mockedReader := new(test.ConsoleReaderMock)
	mockedReader.On("ReadLine").Return("1.0.0", nil)
	config := Init("testdata/build.yaml", &TestConfig{}, mockedReader).(*TestConfig)

	defaultConfig := DefaultConfig(&TestConfig{}).(*TestConfig)
	Expect(config.initialized).To(Equal(true))
	Expect(config.ArmoryURL).To(Equal("http://armory.local"))
	Expect(config.TrebuchetURL).To(Equal(defaultConfig.TrebuchetURL))
	Expect(config.Platforms).To(HaveLen(2))
	Expect(config.Targets).To(HaveLen(1))
	Expect(config.IsParallel).To(Equal(true))
	Expect(config.GetConfigFilePath()).To(Equal("testdata/build.yaml"))
	Expect(config.IsSkipTests).To(Equal("true"))
	Expect(config.OutDir).To(Equal("bin"))
}
