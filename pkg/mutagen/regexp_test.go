package mutagen_test

import (
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/simple-container-com/welder/pkg/util"
)

func TestMatchGroupsWithNames(t *testing.T) {
	RegisterTestingT(t)

	r := regexp.MustCompile(`(?P<prefix>\w+)-(?P<name>[[:alnum:]-]+)-(?P<suffix>\w+)`)

	res := util.MatchGroupsWithNames(r, "prefix-value-123-suffix")

	Expect(res).To(Equal(map[string]string{
		"prefix": "prefix",
		"name":   "value-123",
		"suffix": "suffix",
	}))
}
