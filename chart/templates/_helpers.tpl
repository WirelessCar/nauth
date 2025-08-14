{{/*
Expand the name of the chart.
*/}}
{{- define "nauth.name" -}}
{{- default .Release.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the namespace to use
*/}}
{{- define "nauth.namespaceName" -}}
{{- default .Release.Namespace .Values.namespace.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nauth.serviceAccountName" -}}
{{- default (include "nauth.name" .) .Values.serviceAccount.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "nauth.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nauth.labels" -}}
helm.sh/chart: {{ include "nauth.chart" . }}
{{ include "nauth.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nauth.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nauth.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
