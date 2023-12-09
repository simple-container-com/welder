package render

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/alecthomas/kingpin"
	"github.com/ghodss/yaml"
	rendered "github.com/simple-container-com/welder/pkg/render/rendered"
)

const (
	SchemeTemplate = "template"
	FormatJSON     = "json"
	FormatYAML     = "yaml"
	FormatTable    = "table"
)

var Formats = []string{FormatJSON, FormatYAML}

type OutputFlag struct {
	OutputFormats []string
	Output        string
}

func (o *OutputFlag) Mount(cmd *kingpin.CmdClause) {
	outputformats := append([]string{}, o.OutputFormats...)
	if len(outputformats) == 0 {
		outputformats = append(outputformats, Formats...)
	}
	cmd.Flag("output", fmt.Sprintf("Output format: %v", Formats)).Short('o').PlaceHolder("FORMAT").EnumVar(&o.Output, outputformats...)
}

// Write formats and outputs data in FormatJSON, FormatYAML, or using Go template.
func Write(w io.Writer, f string, data interface{}) error {
	if len(f) == 0 {
		return fmt.Errorf("no format or template specified")
	}

	switch f {
	case FormatJSON:
		return writeJSON(w, data)
	case FormatYAML:
		return writeYAML(w, data)
	// case FormatTable:
	//	return writeTable(w, data)
	default:
		return writeTemplate(w, f, data)
	}
}

func writeTemplate(w io.Writer, f string, data interface{}) error {
	u, err := url.Parse(f)
	if err != nil {
		return fmt.Errorf("bad template URI: %q", f)
	}

	if u.Scheme == SchemeTemplate {
		// we support templates in the PROJECT_ROOT/templates directory
		// without the leading "templates/" path segment
		// e.g. template://service/info/default.tpl
		tpl, err := rendered.Asset(path.Join(u.Host, u.Path))
		if err != nil {
			return fmt.Errorf("missing template: %q", f)
		}

		t := template.Must(template.New("default").Funcs(sprig.TxtFuncMap()).Parse(string(tpl)))
		return t.Execute(w, data)
	}

	return fmt.Errorf("unsupported template URI: %q", f)
}

func GetFile(filePath string) ([]byte, error) {
	return rendered.Asset(filePath)
}

func writeJSON(w io.Writer, data interface{}) error {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(output)
	return err
}

func writeYAML(w io.Writer, data interface{}) error {
	output, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.Write(output)
	return err
}
