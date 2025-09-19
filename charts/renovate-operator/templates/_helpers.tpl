{{ define "renovate-operator.fullname" }}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{ define "renovate-operator.image" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository (ternary .Values.image.tag .Chart.AppVersion (empty .Values.image.tag)) }}
{{- end }}

{{ define "renovate-operator.serviceAccountName" }}
{{- if .Values.serviceAccount.name }}
{{ .Values.serviceAccount.name }}
{{- else }}
{{- include "renovate-operator.fullname" . }}
{{- end }}
{{- end }}
