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

{{/*
Returns the effective auth redirect URL.
Input: dict with keys "redirectUrl" (string) and "Values" (root .Values).
Priority: explicit redirectUrl → route.hostnames[0] → ingress.host.
Scheme: https if ingress.tls is set, http otherwise.
*/}}
{{- define "renovate-operator.authRedirectUrl" -}}
{{- $url := .redirectUrl -}}
{{- if not $url -}}
{{- $host := "" -}}
{{- $values := .Values -}}
{{- if and $values.route.enabled $values.route.hostnames -}}
{{- $host = index $values.route.hostnames 0 -}}
{{- else if $values.ingress.host -}}
{{- $host = $values.ingress.host -}}
{{- end -}}
{{- if $host -}}
{{- $scheme := "http" -}}
{{- if $values.ingress.tls -}}
{{- $scheme = "https" -}}
{{- end -}}
{{- $url = printf "%s://%s/auth/callback" $scheme $host -}}
{{- end -}}
{{- end -}}
{{- $url -}}
{{- end -}}
