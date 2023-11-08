package magebuild

import (
	"crypto/sha256"
	"fmt"
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jstemmer/go-junit-report/formatter"
	"github.com/jstemmer/go-junit-report/parser"
	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/util"
	"golang.org/x/sync/errgroup"
)

var (
	GoBuildEnv = []string{
		"GOPATH=" + build.Default.GOPATH,
	}
)

// CurrentPlatform returns current platform
func (ctx *GoBuildContext) CurrentPlatform() Platform {
	return Platform{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}

// BuildAllPlatforms runs build for all platforms for target
func (ctx *GoBuildContext) BuildAllPlatforms(target Target) error {
	var eg errgroup.Group
	for _, platform := range ctx.Platforms {
		p := platform
		if ctx.IsParallel() {
			eg.Go(func() error {
				return ctx.Build(target, p)
			})
		} else if err := ctx.Build(target, p); err != nil {
			return err
		}
	}
	return eg.Wait()
}

// ForAllPlatforms runs build for all platforms for target
func (ctx *GoBuildContext) ForAllPlatforms(action func(platform Platform) error) error {
	var eg errgroup.Group
	for _, platform := range ctx.ActivePlatforms() {
		p := platform
		buildFnc := func() error {
			return action(p)
		}
		if ctx.IsParallel() {
			eg.Go(buildFnc)
		} else if err := buildFnc(); err != nil {
			return err
		}
	}
	return eg.Wait()
}

// ForAllTargets execute some action for each target
func (ctx *GoBuildContext) ForAllTargets(action func(target Target) error) error {
	var eg errgroup.Group
	for _, target := range ctx.ActiveTargets() {
		t := target
		actionFor := func() error {
			return action(t)
		}
		if ctx.IsParallel() {
			eg.Go(actionFor)
		} else if err := actionFor(); err != nil {
			return err
		}
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}

// TestAll runs go test ./...
func (ctx *GoBuildContext) TestAll(extraArgs ...string) error {
	return ctx.Test("./...", extraArgs...)
}

// Test runs go test
func (ctx *GoBuildContext) Test(target string, extraArgs ...string) error {
	goTestCmd := Cmd{Command: "go test -v " + target + " " + strings.Join(extraArgs, " "),
		Env: ctx.CurrentPlatform().GoEnv(),
	}
	return ctx.TestCommand(goTestCmd)
}

// Test runs go test command specified in the command
func (ctx *GoBuildContext) TestCommand(goTestCmd Cmd) error {
	if !ctx.IsSkipTests() {

		stdoutReader, stdoutWriter := io.Pipe()
		proxyReader, proxyWriter := io.Pipe()
		goTestCmd.Stdout = stdoutWriter

		var eg errgroup.Group
		eg.Go(func() error {
			defer proxyWriter.Close()
			scanner := util.NewLineOrReturnScanner(stdoutReader)
			for {
				if !scanner.Scan() {
					if scanner.Err() != nil {
						return errors.Wrapf(scanner.Err(), "failed to write to proxy writer")
					}
					return nil
				}
				switch scanner.Err() {
				case nil:
					fmt.Println(scanner.Text())
					if _, err := proxyWriter.Write([]byte(scanner.Text() + "\n")); err != nil {
						return errors.Wrapf(err, "failed to write to proxy writer")
					}
				default:
					return errors.Wrapf(scanner.Err(), "failed to read next line from build output")
				}
			}
		})
		eg.Go(func() error {
			report, err := parser.Parse(proxyReader, "")
			if err != nil {
				return errors.Wrapf(err, "failed to parse go tests output")
			}

			junitOutputFile := ctx.JUnitOutputFile
			junitOutputDir := filepath.Dir(junitOutputFile)
			if err := os.MkdirAll(junitOutputDir, os.ModePerm); err != nil {
				return errors.Wrapf(err, "failed to create JUnit output dir: %s", junitOutputDir)
			}
			f, err := os.Create(junitOutputFile)
			if err != nil {
				return errors.Wrapf(err, "failed to create JUnit output file: %s", junitOutputFile)
			}
			defer f.Close()
			if err = formatter.JUnitReportXML(report, false, "", f); err != nil {
				return errors.Wrapf(err, "failed to generate JUnit report")
			}

			return nil
		})

		err := ctx.RunCmd(goTestCmd)
		_ = stdoutWriter.Close()

		svcErr := eg.Wait()

		if err != nil {
			return errors.Wrapf(err, "failed to run tests")
		}
		return svcErr
	}
	return nil
}

// Build runs go build for a certain target and platform
func (ctx *GoBuildContext) Build(target Target, platform Platform, extraArgs ...string) error {
	fmt.Println(fmt.Sprintf("Building %s for %s-%s...", target, platform.GOOS, platform.GOARCH))
	binaryFile := ctx.OutFile(target, platform)
	checksumFile := fmt.Sprintf("%s.sha256", binaryFile)
	extraLdFlags := ""
	if platform.CgoEnabled {
		extraLdFlags = " -linkmode external -extldflags -static"
	}
	extraBuildFlags := ""
	if ctx.ExtraBuildFlags != "-" {
		extraBuildFlags = " " + ctx.ExtraBuildFlags
	}
	if err := ctx.RunCmd(Cmd{Env: platform.GoEnv(),
		Command: "go build -ldflags \"-X main.Version=" + ctx.GetVersion() +
			extraLdFlags + "\" " + extraBuildFlags + " " + strings.Join(extraArgs, " ") +
			" -o \"" + binaryFile + "\" \"" + ctx.SourceFile(target) + "\""}); err != nil {
		return err
	}
	return ctx.WriteFileChecksum(binaryFile, checksumFile)
}

// RunCmd runs any command in the sub shell
func (ctx *GoBuildContext) RunCmd(cmd Cmd) error {
	run := exec.Command("bash", "-c", cmd.Command)
	runString := fmt.Sprintf("%s \"%s\"", run.Path, strings.Join(run.Args[1:], "\" \""))
	if cmd.Wd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		run.Dir = cwd
	} else {
		run.Dir = cmd.Wd
	}
	run.Stdout = cmd.Stdout
	if run.Stdout == nil {
		run.Stdout = os.Stdout
	}
	run.Stderr = cmd.Stderr
	if run.Stderr == nil {
		run.Stderr = os.Stderr
	}
	run.Stdin = cmd.Stdin
	if run.Stdin == nil {
		run.Stdin = os.Stdin
	}
	run.Env = os.Environ()
	for _, env := range cmd.Env {
		run.Env = append(run.Env, env)
	}
	fmt.Println(fmt.Sprintf("Executing '%s'", runString))
	err := run.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to execute '%s'", runString)
	}
	return nil
}

// GenerateSwaggerClient generates swagger client from URL into relative path
func (ctx *GoBuildContext) GenerateSwaggerClient(relPath string, swaggerURL string, appName string) error {
	fmt.Println(fmt.Sprintf("Generating %s's client from %s to %s...", appName, swaggerURL, relPath))
	basePath := ctx.Path(relPath)
	if err := os.MkdirAll(basePath, os.ModePerm); err != nil {
		return err
	}
	swaggerFile := filepath.Join(basePath, "swagger.json")
	generateCmd := fmt.Sprintf("curl -s '%s' | jq -S . > '%s'", swaggerURL, swaggerFile)
	if _, err := exec.LookPath("jq"); err != nil {
		// no jq available, generating without sorting
		generateCmd = fmt.Sprintf("curl -s '%s' > '%s'", swaggerURL, swaggerFile)
	}
	if err := ctx.RunCmd(Cmd{Command: generateCmd}); err != nil {
		return err
	}
	return ctx.RunCmd(Cmd{Command: fmt.Sprintf(
		"go run '%s' generate client -f '%s' --skip-validation -t '%s' -A %s",
		ctx.VendorPath("github.com/go-swagger/go-swagger/cmd/swagger"), swaggerFile, basePath, appName),
		Env: ctx.CurrentPlatform().GoEnv()})
}

// ProjectRoot finds git root traversing from the current directory up to the dir with .git dir
func (ctx *GoBuildContext) ProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	configFileName := filepath.Base(ctx.GetConfigFilePath())
	_, err = os.Stat(filepath.Join(cwd, configFileName))
	for os.IsNotExist(err) && filepath.Dir(cwd) != "/" {
		cwd = filepath.Dir(cwd)
		_, err = os.Stat(filepath.Join(cwd, configFileName))
	}
	if filepath.Dir(cwd) == "/" {
		panic("Could not determine project root! Make sure to run build from the root!")
	}
	return cwd
}

// GitRoot finds git root traversing from the current directory up to the dir with .git dir
func (ctx *GoBuildContext) GitRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	_, err = os.Stat(filepath.Join(cwd, ".git"))
	for os.IsNotExist(err) && filepath.Dir(cwd) != "/" {
		cwd = filepath.Dir(cwd)
		_, err = os.Stat(filepath.Join(cwd, ".git"))
	}
	if filepath.Dir(cwd) == "/" {
		panic("Could not determine Git root for the project")
	}
	return cwd
}

// Target returns target by its name defined in config
func (ctx *GoBuildContext) Target(name string) Target {
	for _, target := range ctx.Targets {
		if target.Name == name {
			return target
		}
	}
	panic("Failed to find target with name: " + name)
}

// Tarball builds tarball for target and platform
func (ctx *GoBuildContext) BuildBundle(tarball Bundle) error {
	if err := ctx.WriteFileChecksum(tarball.TargetBinaryFile, tarball.TargetChecksumFile); err != nil {
		return err
	}
	chDirTo := filepath.Dir(tarball.TargetBinaryFile)
	binaryFileName := filepath.Base(tarball.TargetBinaryFile)
	checksumFileName := filepath.Base(tarball.TargetChecksumFile)
	bundleCmd := "tar -C " + chDirTo + " -czf " + tarball.TarFile + " " + binaryFileName + " " + checksumFileName
	if err := ctx.RunCmd(Cmd{Command: bundleCmd}); err != nil {
		return err
	}

	if err := ctx.WriteFileChecksum(tarball.TarFile, tarball.TarChecksumFile); err != nil {
		return err
	}

	return nil
}

// BundleFile built file path for target
func (ctx *GoBuildContext) BundleFileName(target Target, platform Platform) string {
	return filepath.Join(ctx.OutDir, fmt.Sprintf("%s-%s.tar.gz", target.Name, platform.String()))
}

// Tarball returns resulting tarball file names for target and platform
func (ctx *GoBuildContext) Bundle(target Target, platform Platform) Bundle {
	tarName := ctx.BundleFileName(target, platform)
	checksumName := fmt.Sprintf("%s.sha256", tarName)
	targetBinaryFile := ctx.OutFile(target, platform)
	targetChecksumFile := fmt.Sprintf("%s.sha256", targetBinaryFile)
	return Bundle{Target: target, Platform: platform,
		TargetBinaryFile: targetBinaryFile, TargetChecksumFile: targetChecksumFile,
		TarFile: tarName, TarChecksumFile: checksumName,
	}
}

// Publish uploads bundle to Statlas
func (ctx *GoBuildContext) Publish(bundle Bundle, version string) error {
	targetUploadPath := ctx.BaseTargetURL(bundle.Target)
	return ctx.UploadFileToStatlas(fmt.Sprintf("%s/releases/%s", targetUploadPath, version),
		fmt.Sprintf("%s-%s.tar.gz", bundle.Platform.GOOS, bundle.Platform.GOARCH),
		bundle.TarFile, bundle.TarChecksumFile)
}

// UploadFileToStatlas uploads file along with hashsum file to statlas at given base url
func (ctx *GoBuildContext) UploadFileToStatlas(
	baseURL string, baseFileName string, file string, checksumFile string,
) error {
	statlasTokenHeader := fmt.Sprintf("authorization: Token %s", ctx.StatlasToken)
	if ctx.SlauthToken != "-" {
		statlasTokenHeader = fmt.Sprintf("authorization: Slauth %s", ctx.SlauthToken)
	} else if ctx.BambooJWTToken != "-" {
		statlasTokenHeader = fmt.Sprintf("authorization: Bearer %s", ctx.BambooJWTToken)
	}
	filesToUpload := map[string]string{
		file:         fmt.Sprintf("%s/%s", baseURL, baseFileName),
		checksumFile: fmt.Sprintf("%s/%s.sha256", baseURL, baseFileName),
	}
	for source, target := range filesToUpload {
		fmt.Println("\tUploading ", target, "...")

		if err := ctx.RunCmd(Cmd{Command: "curl -f -sS -X PUT -H '" + statlasTokenHeader + "' -T " + source + " " + target}); err != nil {
			return err
		}
	}
	return nil
}

// ManifestTargetURL return base URL where to publish manifest file
func (ctx *GoBuildContext) ManifestTargetURL(target Target) string {
	fmt.Println(ctx.StatlasURL)
	return fmt.Sprintf("%s/%s", ctx.BaseTargetURL(target), "manifest.toml")
}

// BaseTargetURL return base URL where to publish target plugin
func (ctx *GoBuildContext) BaseTargetURL(target Target) string {
	return fmt.Sprintf("%s/atlas-cli-plugin-%s", ctx.StatlasURL, target.Name)
}

// BundleFile built file path for target
func (ctx *GoBuildContext) BundleFile(target Target, platform Platform) string {
	return filepath.Join(ctx.OutFileDir(target, platform), fmt.Sprintf("%s-%s-%s.tar.gz", target.Name, platform.GOOS, platform.GOARCH))
}

// OutFile built file path for target
func (ctx *GoBuildContext) OutFile(target Target, platform Platform) string {
	return filepath.Join(ctx.OutFileDir(target, platform), ctx.OutFileName(target, platform))
}

// OutFileDir returns directory for output file
func (ctx *GoBuildContext) OutFileDir(target Target, platform Platform) string {
	return filepath.Join(ctx.OutPath(), fmt.Sprintf("%s-%s", platform.GOOS, platform.GOARCH))
}

// OutFileName file name based for target based on platform
func (ctx *GoBuildContext) OutFileName(target Target, platform Platform) string {
	var osExt string
	if platform.IsWindows() {
		osExt = ".exe"
	}

	return fmt.Sprintf("%s%s", target.Name, osExt)
}

// OutPath returns full path to output directory
func (ctx *GoBuildContext) OutPath(pathTo ...string) string {
	return filepath.Join(append([]string{ctx.ProjectRoot(), ctx.OutDir}, pathTo...)...)
}

// SourceFile returns path to source file for the target
func (ctx *GoBuildContext) SourceFile(target Target) string {
	return filepath.Join(ctx.ProjectRoot(), target.Path)
}

// ManifestFile returns path to manifest file for the target
func (ctx *GoBuildContext) ManifestFile(target Target) string {
	return filepath.Join(ctx.ProjectRoot(), filepath.Dir(target.Path), "manifest.toml")
}

// Path returns full path from project root
func (ctx *GoBuildContext) Path(relPath string) string {
	return filepath.Join(ctx.ProjectRoot(), relPath)
}

// VendorPath returns full path to a file/dir in /vendor dir
func (ctx *GoBuildContext) VendorPath(relPath string) string {
	return filepath.Join(ctx.ProjectRoot(), "vendor", relPath)
}

// GoEnv returns environment variables necessary for Go to run properly
func (p Platform) GoEnv(extraEnv ...string) []string {
	res := make([]string, len(GoBuildEnv)+2)
	for _, env := range GoBuildEnv {
		res = append(res, env)
	}
	res = append(res, "GOOS="+p.GOOS)
	res = append(res, "GOARCH="+p.GOARCH)
	if p.CgoEnabled {
		res = append(res, "CGO_ENABLED=1")
	} else {
		res = append(res, "CGO_ENABLED=0")
	}
	for _, env := range extraEnv {
		res = append(res, env)
	}
	return res
}

// ListAllSubDirs returns list of all subdirectories under the directory
func (ctx GoBuildContext) ListAllSubDirs(dir string) ([]string, error) {
	var paths []string
	if err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if info.IsDir() && path != dir {
				relPath, err := filepath.Rel(dir, path)
				if err != nil {
					return err
				}
				paths = append(paths, relPath)
			}
			return nil
		}); err != nil {
		return paths, err
	}
	return paths, nil
}

func (p *Platform) IsWindows() bool {
	return p.GOOS == "windows"
}

func (ctx GoBuildContext) PlatformsToVariants() []string {
	res := make([]string, len(ctx.Platforms))
	for i, p := range ctx.Platforms {
		res[i] = p.String()
	}
	return res
}

func (ctx GoBuildContext) WriteFileChecksum(inputFile string, outputFile string) error {
	f, err := os.Open(inputFile)

	if err != nil {
		return err
	}

	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}

	fHash, err := os.Create(outputFile)

	if err != nil {
		return err
	}

	defer fHash.Close()

	_, err = fHash.Write([]byte(fmt.Sprintf("%x", hasher.Sum(nil))))

	return err
}
