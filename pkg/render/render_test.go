package render

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ServiceInfo struct {
	Name             string   `json:"name,omitempty"`
	Owner            string   `json:"owner,omitempty"`
	Email            string   `json:"email,omitempty"`
	Logs             string   `json:"logs,omitempty"`
	SSAMContainerURL string   `json:"ssamContainerURL,omitempty"`
	MicroscopeURL    string   `json:"microscopeURL,omitempty"`
	PagerdutyURL     string   `json:"pagerdutyURL,omitempty"`
	BusinessUnit     string   `json:"businessUnit,omitempty"`
	Deployments      []string `json:"deployments,omitempty"`
}

func TestExportedValues(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "json", FormatJSON)
	assert.Equal(t, "yaml", FormatYAML)
	assert.Equal(t, []string{"json", "yaml"}, Formats)
}

func TestWrite(t *testing.T) {
	t.Parallel()

	d := ServiceInfo{
		Name:             "foo",
		Owner:            "nobody",
		Email:            "nobody@atlassian.com",
		SSAMContainerURL: "https://ssam.foo/foo--foo",
		MicroscopeURL:    "https://microscope.foo/foo",
		PagerdutyURL:     "https://pagerduty.foo/PFOOFAKE",
		Logs:             "https://logs",
	}

	type testCase struct {
		format  string
		want    string
		wantErr error
	}
	cases := []testCase{
		{
			format:  "newscheme://is/not/supported.tpl",
			want:    "",
			wantErr: errors.New("unsupported template URI: \"newscheme://is/not/supported.tpl\""),
		},
		{
			format:  "template://does/not/exist.tpl",
			want:    "",
			wantErr: errors.New("missing template: \"template://does/not/exist.tpl\""),
		},
		{
			format:  "template://test/service/info.tpl",
			want:    "Name:           foo\nOwner:          nobody <nobody@atlassian.com>\nBusiness Unit:  \nLogs:           https://logs\n  For more info on logs, see go/micros2-logs\n\nMicroscope:     https://microscope.foo/foo\nPagerduty:      https://pagerduty.foo/PFOOFAKE\nSSAM Container: https://ssam.foo/foo--foo\n\n",
			wantErr: nil,
		},
		{
			format:  FormatJSON,
			want:    "{\n  \"name\": \"foo\",\n  \"owner\": \"nobody\",\n  \"email\": \"nobody@atlassian.com\",\n  \"logs\": \"https://logs\",\n  \"ssamContainerURL\": \"https://ssam.foo/foo--foo\",\n  \"microscopeURL\": \"https://microscope.foo/foo\",\n  \"pagerdutyURL\": \"https://pagerduty.foo/PFOOFAKE\"\n}",
			wantErr: nil,
		},
		{
			format:  FormatYAML,
			want:    "email: nobody@atlassian.com\nlogs: https://logs\nmicroscopeURL: https://microscope.foo/foo\nname: foo\nowner: nobody\npagerdutyURL: https://pagerduty.foo/PFOOFAKE\nssamContainerURL: https://ssam.foo/foo--foo\n",
			wantErr: nil,
		},
	}
	for i, c := range cases {
		ti := i
		tc := c
		t.Run(fmt.Sprintf("[%d]", ti), func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			err := Write(&b, tc.format, d)
			if tc.wantErr != nil {
				assert.EqualError(t, tc.wantErr, err.Error())
				return
			}
			require.NoError(t, err)

			got := b.String()
			assert.Equal(t, tc.want, got)
		})
	}
}
