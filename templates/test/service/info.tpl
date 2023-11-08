Name:           {{.Name}}
Owner:          {{.Owner}} <{{.Email}}>
Business Unit:  {{.BusinessUnit}}
Logs:           {{.Logs}}
  For more info on logs, see go/micros2-logs

Microscope:     {{.MicroscopeURL}}
Pagerduty:      {{.PagerdutyURL}}
SSAM Container: {{.SSAMContainerURL}}
{{ if .Deployments }}Bamboo:{{ range .Deployments }}
- {{.}}
{{end}}{{end}}
