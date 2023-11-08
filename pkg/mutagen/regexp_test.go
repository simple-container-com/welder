package mutagen_test

import (
	. "github.com/onsi/gomega"
	"github.com/smecsia/welder/pkg/util"
	"regexp"
	"testing"
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
