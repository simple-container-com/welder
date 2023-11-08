{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "generator.name" -}}
{{- default $.Chart.Name $.Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "generator.imagePullSecret" }}
{{- $registry := .Values.docker.registry }}
{{- $username := .Values.docker.username }}
{{- $password := .Values.docker.password }}
{{- $auth := (printf "%s:%s" $username $password | b64enc) }}
{{- printf "{\"auths\":{\"%s\":{\"auth\": \"%s\",\"username\":\"%s\",\"password\":\"%s\"}}}" $registry $auth $username $password | b64enc }}
{{- end }}


{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "generator.fullname" -}}
{{- if $.Values.fullnameOverride }}
{{- $.Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name $.Values.nameOverride }}
{{- if contains $name $.Release.Name }}
{{- $.Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" $.Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "generator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "generator.labels" -}}
helm.sh/chart: {{ include "generator.chart" . }}
{{ include "generator.selectorLabels" . }}
{{- if $.Chart.AppVersion }}
app.kubernetes.io/version: {{ $.Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ $.Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "generator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "generator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Renders a value that contains template.
Usage:
{{ include "generator.tplValue" ( dict "value" .Values.path.to.the.Value "context" $) }}
*/}}
{{- define "generator.tplValue" -}}
    {{- if typeIs "string" .value }}
        {{- tpl .value .context }}
    {{- else }}
        {{- tpl (.value | toYaml) .context }}
    {{- end }}
{{- end -}}
