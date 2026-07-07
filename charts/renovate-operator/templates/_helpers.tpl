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
Validates a scheme override value. Input: dict with keys "scheme" and "key" (values path for the error message).
*/}}
{{- define "renovate-operator.validateScheme" -}}
{{- if and .scheme (not (has .scheme (list "http" "https"))) -}}
{{- fail (printf "%s must be either \"http\" or \"https\", got %q" .key .scheme) -}}
{{- end -}}
{{- end -}}

{{/*
Returns the effective auth redirect URL.
Input: dict with keys "redirectUrl" (string), "redirectScheme" (string) and "Values" (root .Values).
Priority: explicit redirectUrl → route.hostnames[0] → ingress.host.
Scheme: redirectScheme if set, https if ingress.tls is set, http otherwise.
*/}}
{{- define "renovate-operator.authRedirectUrl" -}}
{{- include "renovate-operator.validateScheme" (dict "scheme" .redirectScheme "key" "auth.*.redirectScheme") -}}
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
{{- if .redirectScheme -}}
{{- $scheme = .redirectScheme -}}
{{- end -}}
{{- $url = printf "%s://%s/auth/callback" $scheme $host -}}
{{- end -}}
{{- end -}}
{{- $url -}}
{{- end -}}

{{/*
Returns the external base URL of the webhook server. When unifiedWebhookHost
is true the UI ingress/route values are used; otherwise the webhook-specific
values are used. Priority within each: route.hostnames[0] (https) →
ingress.host (https when tls is set, http otherwise). webhook.baseUrlScheme
overrides the detected scheme. Empty when not exposed.
*/}}
{{- define "renovate-operator.webhookBaseUrl" -}}
{{- $override := .Values.webhook.baseUrlScheme -}}
{{- include "renovate-operator.validateScheme" (dict "scheme" $override "key" "webhook.baseUrlScheme") -}}
{{- $v := .Values.webhook -}}
{{- if .Values.webhook.unifiedWebhookHost -}}
{{- $v = .Values -}}
{{- end -}}
{{- $url := "" -}}
{{- if and $v.route.enabled $v.route.hostnames -}}
{{- $url = printf "%s://%s" (default "https" $override) (index $v.route.hostnames 0) -}}
{{- else if and $v.ingress.enabled $v.ingress.host -}}
{{- $scheme := "http" -}}
{{- if $v.ingress.tls -}}
{{- $scheme = "https" -}}
{{- end -}}
{{- $url = printf "%s://%s" (default $scheme $override) $v.ingress.host -}}
{{- end -}}
{{- $url -}}
{{- end -}}
