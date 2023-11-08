package template

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestExpressionEval(t *testing.T) {
	RegisterTestingT(t)
	data := map[string]interface{}{
		"mode":                   "sox",
		"profile:bamboo.enabled": "true",
	}
	parsedTpl := NewTemplate().WithData(data)

	result, err := parsedTpl.EvalToBool(`'${mode}' == 'sox' && ${profile:bamboo.enabled}`)
	Expect(err).To(BeNil())
	Expect(result).To(BeTrue())
}
