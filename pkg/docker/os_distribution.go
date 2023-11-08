package docker

import (
	"fmt"
	"regexp"
)

type OSDistribution interface {
	Name() string
	IsLinuxBased() bool
	InstallPackageCommands(packageName string, cmdSuffix string) string
}

// OSDistributionImpl represents the OS distribution
type OSDistributionImpl struct {
	distName string
}

const (
	OSDistributionAlpine  = "alpine"
	OSDistributionCentos  = "centos"
	OSDistributionUbuntu  = "ubuntu"
	OSDistributionDebian  = "debian"
	OSDistributionRhel    = "rhel"
	OSDistributionFedora  = "fedora"
	OSDistributionFlatcar = "flatcar" // not supported
	OSDistributionAmazon  = "amazon"  // not supported
	OSDistributionOracle  = "oracle"  // not supported
	OSDistributionUnknown = "unknown"
)

var packageNameAlternatives = map[string]map[string]string{
	"openssh-client": {
		OSDistributionRhel:   "openssh",
		OSDistributionCentos: "openssh",
		OSDistributionFedora: "openssh",
	},
}

var UnknownOSDistribution = OSDistributionImpl{OSDistributionUnknown}

func OSReleaseOutputToDistribution(output string) OSDistribution {
	re := regexp.MustCompile("(?m)^ID=\"?(\\w+)\"?")
	match := re.FindStringSubmatch(output)
	if len(match) == 0 {
		return &OSDistributionImpl{OSDistributionUnknown}
	}
	return &OSDistributionImpl{match[1]}
}

func (o OSDistributionImpl) Name() string {
	return o.distName
}

func (o OSDistributionImpl) IsLinuxBased() bool {
	return o.distName != OSDistributionUnknown
}

func (o OSDistributionImpl) InstallPackageCommands(packageName string, cmdSuffix string) string {
	if alternatives, ok := packageNameAlternatives[packageName]; ok {
		if alternative, ok := alternatives[o.distName]; ok {
			packageName = alternative
		}
	}
	switch o.distName {
	case OSDistributionDebian, OSDistributionUbuntu:
		return o.installCmdDebian(packageName, cmdSuffix)
	case OSDistributionAlpine:
		return fmt.Sprintf("apk add --update %s %s || true", packageName, cmdSuffix)
	case OSDistributionCentos:
	case OSDistributionFedora:
	case OSDistributionRhel:
		return fmt.Sprintf("yum makecache %s || true; yum -y install %s %s || true; "+
			"dnf install %s %s || true; "+
			"microdnf install -y %s %s || true",
			cmdSuffix, packageName, cmdSuffix, packageName, cmdSuffix, packageName, cmdSuffix)
	}
	return o.installCmdDebian(packageName, cmdSuffix)
}

func (o OSDistributionImpl) installCmdDebian(packageName string, cmdSuffix string) string {
	return fmt.Sprintf("apt-get update %s || true; apt-get install --no-install-recommends -y %s %s || true",
		cmdSuffix, packageName, cmdSuffix)
}
