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

{{- define "renovate-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.name -}}
{{- .Values.serviceAccount.name -}}
{{- else -}}
{{- include "renovate-operator.fullname" . -}}
{{- end -}}
{{- end -}}

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

{{/*
Returns the external base URL of the webhook server, derived from the chart's
webhook exposure. Priority: webhook.route.hostnames[0] (https, TLS terminates
at the Gateway) → webhook.ingress.host (https when webhook.ingress.tls is set,
http otherwise). Empty when the webhook is not exposed by this chart.
*/}}
{{- define "renovate-operator.webhookBaseUrl" -}}
{{- $url := "" -}}
{{- if and .Values.webhook.route.enabled .Values.webhook.route.hostnames -}}
{{- $url = printf "https://%s" (index .Values.webhook.route.hostnames 0) -}}
{{- else if and .Values.webhook.ingress.enabled .Values.webhook.ingress.host -}}
{{- $scheme := "http" -}}
{{- if .Values.webhook.ingress.tls -}}
{{- $scheme = "https" -}}
{{- end -}}
{{- $url = printf "%s://%s" $scheme .Values.webhook.ingress.host -}}
{{- end -}}
{{- $url -}}
{{- end -}}
