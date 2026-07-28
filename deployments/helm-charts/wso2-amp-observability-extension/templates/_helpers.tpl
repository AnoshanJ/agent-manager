{{/*
Expand the name of the chart.
*/}}
{{- define "amp-observability-extension.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "amp-observability-extension.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "amp-observability-extension.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "amp-observability-extension.labels" -}}
helm.sh/chart: {{ include "amp-observability-extension.chart" . }}
{{ include "amp-observability-extension.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "amp-observability-extension.selectorLabels" -}}
app.kubernetes.io/name: {{ include "amp-observability-extension.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
OAuth 2.0 authorization servers advertised in RFC 9728 protected resource
metadata. Defaults to the token issuer: both name the same Thunder deployment,
and advertising a different URL than the one tokens are validated against sends
MCP clients to an authorization server whose tokens this service then rejects.
*/}}
{{- define "amp-observability-extension.authorizationServers" -}}
{{- .Values.amObserver.oauth.authorizationServers | default .Values.amObserver.auth.issuer -}}
{{- end }}

{{/*
Accepted token audiences. publicUrl — with the trailing slash Thunder stamps on
RFC 8707 resource identifiers — is appended so tokens minted for this service's
own MCP client stay valid when publicUrl is overridden, without the operator
having to restate the whole audience list.
*/}}
{{- define "amp-observability-extension.audience" -}}
{{- $audiences := .Values.amObserver.auth.audience | splitList "," | compact -}}
{{- if .Values.amObserver.publicUrl -}}
{{- $resource := printf "%s/" (trimSuffix "/" .Values.amObserver.publicUrl) -}}
{{- if not (has $resource $audiences) -}}
{{- $audiences = append $audiences $resource -}}
{{- end -}}
{{- end -}}
{{- join "," $audiences -}}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "amp-observability-extension.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "amp-observability-extension.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
