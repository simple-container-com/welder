package docker

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestDetectOSDistributionByOutput(t *testing.T) {
	RegisterTestingT(t)

	Expect(OSReleaseOutputToDistribution(`NAME="CentOS Linux"
VERSION="7 (Core)"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="7"
PRETTY_NAME="CentOS Linux 7 (Core)"
ANSI_COLOR="0;31"
CPE_NAME="cpe:/o:centos:centos:7"
HOME_URL="https://www.centos.org/"
BUG_REPORT_URL="https://bugs.centos.org/"

CENTOS_MANTISBT_PROJECT="CentOS-7"
CENTOS_MANTISBT_PROJECT_VERSION="7"
REDHAT_SUPPORT_PRODUCT="centos"
REDHAT_SUPPORT_PRODUCT_VERSION="7"
`).Name()).To(Equal(OSDistributionCentos))

	Expect(OSReleaseOutputToDistribution(`NAME="Red Hat Enterprise Linux"
VERSION="9.0 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.0"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux 9.0 (Plow)"
ANSI_COLOR="0;31"
LOGO="fedora-logo-icon"
CPE_NAME="cpe:/o:redhat:enterprise_linux:9::baseos"
HOME_URL="https://www.redhat.com/"
DOCUMENTATION_URL="https://access.redhat.com/documentation/red_hat_enterprise_linux/9/"
BUG_REPORT_URL="https://bugzilla.redhat.com/"

REDHAT_BUGZILLA_PRODUCT="Red Hat Enterprise Linux 9"
REDHAT_BUGZILLA_PRODUCT_VERSION=9.0
REDHAT_SUPPORT_PRODUCT="Red Hat Enterprise Linux"
REDHAT_SUPPORT_PRODUCT_VERSION="9.0"
`).Name()).To(Equal(OSDistributionRhel))
	Expect(OSReleaseOutputToDistribution(`PRETTY_NAME="Debian GNU/Linux 10 (buster)"
NAME="Debian GNU/Linux"
VERSION_ID="10"
VERSION="10 (buster)"
VERSION_CODENAME=buster
ID=debian
HOME_URL="https://www.debian.org/"
SUPPORT_URL="https://www.debian.org/support"
BUG_REPORT_URL="https://bugs.debian.org/"`).Name()).To(Equal(OSDistributionDebian))
	Expect(OSReleaseOutputToDistribution(`NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.12.1
PRETTY_NAME="Alpine Linux v3.12"
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://bugs.alpinelinux.org/"`).Name()).To(Equal(OSDistributionAlpine))
	Expect(OSReleaseOutputToDistribution(`ID=alpine
VERSION_ID=3.12.1"`).Name()).To(Equal(OSDistributionAlpine))
	Expect(OSReleaseOutputToDistribution(`NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.12.1
PRETTY_NAME="Alpine Linux v3.12"
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://bugs.alpinelinux.org/"`).Name()).To(Equal(OSDistributionAlpine))
	Expect(OSReleaseOutputToDistribution(`ID=alpine
VERSION_ID=3.12.1"`).Name()).To(Equal(OSDistributionAlpine))
}
