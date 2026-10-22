# renovate-operator

![Version: 6.6.0](https://img.shields.io/badge/Version-6.6.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 6.6.0](https://img.shields.io/badge/AppVersion-6.6.0-informational?style=flat-square)

Chart to deploy the renovate operator to your cluster

**Homepage:** <https://mogenius.com>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Lukas Hankeln @mogenius | <lukas@mogenius.com> | <https://mogenius.com> |

## Source Code

* <https://github.com/mogenius/renovate-operator>
* <https://helm.mogenius.com/public>
* <oci://ghcr.io/mogenius/helm-charts/renovate-operator>

## Requirements

Kubernetes: `>=1.29.0-0`

| Repository | Name | Version |
|------------|------|---------|
| https://valkey.io/valkey-helm/ | valkey | 0.12.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | affinity for the operator pod |
| auth.defaultAllowedGroups | string | `""` | DEPRECATED: use authorization.defaults.adminGroups. Comma-separated list of default groups granted full access to RenovateJobs without explicit access configuration |
| auth.github.clientId | string | `""` | GitHub OAuth App client ID |
| auth.github.enabled | bool | `false` | whether to enable GitHub OAuth authentication for the UI (mutually exclusive with OIDC) |
| auth.github.existingSecret | string | `""` | name of an existing secret containing the GitHub OAuth client secret |
| auth.github.orgGroups | bool | `false` | optional: map GitHub org and team membership to session groups as "org" and "org/team". Requires the read:org scope, so every user must re-consent |
| auth.github.redirectScheme | string | `""` | optional: override the scheme (http or https) of the auto-detected redirect URL, e.g. when TLS terminates at an external load balancer or Gateway (ignored when redirectUrl is set) |
| auth.github.redirectUrl | string | `""` | optional: GitHub OAuth redirect URL (auto-detected from ingress host if empty) |
| auth.github.secretKey | string | `"client-secret"` | key in the existing secret that contains the GitHub OAuth client secret |
| auth.github.sessionSecretKey | string | `""` | optional: session encryption secret (auto-generated if empty, but sessions won't survive pod restarts) |
| auth.oidc.additionalScopes | list | `[]` | optional: additional OIDC scopes to request (e.g., "groups" for Keycloak) |
| auth.oidc.allowedGroupPattern | string | `""` | Matched case-insensitively, since group names are normalized to lowercase. |
| auth.oidc.allowedGroupPrefix | string | `""` | Matched case-insensitively, since group names are normalized to lowercase. |
| auth.oidc.caCert.existingSecret | string | `""` | name of an existing secret containing a PEM-encoded CA certificate to trust for OIDC TLS connections |
| auth.oidc.caCert.key | string | `"ca.crt"` | key in the secret that contains the CA certificate |
| auth.oidc.caCert.mountPath | string | `"/etc/ssl/certs/oidc-ca.crt"` | mount path for the CA certificate inside the operator container |
| auth.oidc.clientId | string | `""` | OIDC client ID |
| auth.oidc.enabled | bool | `false` | whether to enable OIDC authentication for the UI |
| auth.oidc.existingSecret | string | `""` | name of an existing secret containing the OIDC client secret |
| auth.oidc.fetchUserInfoGroups | bool | `false` | optional: fetch groups from the OIDC userinfo endpoint and merge with ID token groups (for providers that only expose groups via userinfo) |
| auth.oidc.groupsClaim | string | `""` | Set this if your provider emits groups under a namespaced claim, e.g. "https://example.com/groups". |
| auth.oidc.insecureSkipVerify | bool | `false` | optional: skip TLS verification for the OIDC provider (not recommended for production) |
| auth.oidc.issuerUrl | string | `""` | OIDC issuer URL (e.g. https://accounts.google.com or https://login.microsoftonline.com/{tenant}/v2.0) |
| auth.oidc.logoutUrl | string | `""` | optional: OIDC end_session_endpoint URL for provider logout (auto-discovered if available) |
| auth.oidc.pkceEnabled | bool | `true` | optional: enable PKCE (Proof Key for Code Exchange) for the OIDC auth flow (enabled by default) |
| auth.oidc.redirectScheme | string | `""` | optional: override the scheme (http or https) of the auto-detected redirect URL, e.g. when TLS terminates at an external load balancer or Gateway (ignored when redirectUrl is set) |
| auth.oidc.redirectUrl | string | `""` | optional: OIDC redirect URL (auto-detected from ingress host if empty) |
| auth.oidc.secretKey | string | `"client-secret"` | key in the existing secret that contains the OIDC client secret |
| auth.oidc.sessionSecretKey | string | `""` | optional: session encryption secret (auto-generated if empty, but sessions won't survive pod restarts) |
| authorization.defaults | object | `{"adminGroups":[],"adminUsers":[],"anonymousRead":false,"anonymousReadLogs":false,"readerGroups":[],"readerUsers":[]}` | adding to it, so a per-job spec.access can narrow access as well as widen it. |
| authorization.defaults.adminGroups | list | `[]` | groups granted full access (view plus trigger, cancel and discovery) |
| authorization.defaults.adminUsers | list | `[]` | individual accounts granted full access, by email or username |
| authorization.defaults.anonymousRead | bool | `false` | whether RenovateJobs are readable without a session (public dashboard) |
| authorization.defaults.anonymousReadLogs | bool | `false` | whether visitors holding only anonymous read access may stream Renovate logs (logs are unredacted) |
| authorization.defaults.readerGroups | list | `[]` | groups granted read-only access (view jobs, projects and logs, no actions) |
| authorization.defaults.readerUsers | list | `[]` | individual accounts granted read-only access, by email or username |
| authorization.enabled | bool | `true` | request is an admin in that case. |
| authorization.suspendFromUI | bool | `false` | See docs/operations/suspend.md. |
| basePath | string | `""` | with your identity provider must include this sub-path. |
| config.defaultJobActiveDeadlineSeconds | int | `1800` | default active deadline seconds for renovate jobs |
| config.defaultJobBackoffLimit | int | `1` | default backoff limit for renovate jobs |
| config.deleteSuccessfulJobs | bool | `false` | whether to delete successfully finished renovate jobs (will lose logs) |
| config.forwardCacheToJobs | bool | `true` | whether to forward valkey cache to renovate jobs - valkey needs to be configured |
| config.forwardS3CacheToJobs | bool | `true` | whether to forward S3 cache configuration to Renovate executor jobs - s3.bucket needs to be set |
| config.globalParallelismLimit | int | `0` | global limit for concurrent renovate executor jobs across all RenovateJobs (0 = unlimited) |
| config.jobTTLSecondsAfterFinished | int | `-1` | TTL for finished renovate jobs in seconds, -1 means they are kept forever |
| config.logStorage.mode | string | `"disabled"` | log store mode: "disabled" (default, no-op), "memory" (in-process), "valkey" (needs valkey enabled), or "s3" (needs s3.bucket set) |
| config.podLabelTemplates | object | `{}` | collapsed automatically, e.g. "{jobType}-{job}-{project}" -> "discovery-myjob" when project is empty. |
| crd.image.imagePullSecrets | list | `[]` | imagePullSecrets for the operator pod |
| crd.image.pullPolicy | string | `"IfNotPresent"` |  |
| crd.image.registry | string | `"registry.k8s.io"` |  |
| crd.image.repository | string | `"kubectl"` |  |
| crd.image.tag | string | `"v1.37.1"` |  |
| crd.install | bool | `true` | whether to install the CRD or not, set to false if you want to install the CRD separately |
| crd.mode | string | `"hook"` |  |
| crd.securityContext.container.allowPrivilegeEscalation | bool | `false` |  |
| crd.securityContext.container.capabilities.drop[0] | string | `"ALL"` |  |
| crd.securityContext.container.privileged | bool | `false` |  |
| crd.securityContext.container.readOnlyRootFilesystem | bool | `true` |  |
| crd.securityContext.container.runAsGroup | int | `1000` |  |
| crd.securityContext.container.runAsNonRoot | bool | `true` |  |
| crd.securityContext.container.runAsUser | int | `1000` |  |
| crd.securityContext.container.seccompProfile.type | string | `"RuntimeDefault"` |  |
| crd.securityContext.pod.fsGroup | int | `1000` |  |
| crd.securityContext.pod.runAsGroup | int | `1000` |  |
| crd.securityContext.pod.runAsNonRoot | bool | `true` |  |
| crd.securityContext.pod.runAsUser | int | `1000` |  |
| crd.securityContext.pod.seccompProfile.type | string | `"RuntimeDefault"` |  |
| deployment.annotations | object | `{}` | additional annotations for the operator deployment |
| deployment.labels | object | `{}` | additional labels for the operator deployment |
| deployment.podAnnotations | object | `{}` | additional annotations for the operator pods |
| deployment.podLabels | object | `{}` | additional labels for the operator pods |
| deployment.revisionHistoryLimit | int | `10` | revision history limit for the operator deployment |
| externalKeyValueStore | object | `{"cluster":false,"existingSecret":{"hostKey":null,"name":null,"passwordKey":null,"portKey":null,"urlKey":null,"usernameKey":null},"host":null,"port":6379,"useTls":false,"username":null}` | Active when host or existingSecret.name is set. |
| externalKeyValueStore.cluster | bool | `false` | whether to connect to a cluster (redis+cluster://). All usages then share database 0. |
| externalKeyValueStore.existingSecret | object | `{"hostKey":null,"name":null,"passwordKey":null,"portKey":null,"urlKey":null,"usernameKey":null}` | Keys win over their clear-value counterparts above. |
| externalKeyValueStore.existingSecret.hostKey | string | `nil` | key containing the host for the connection |
| externalKeyValueStore.existingSecret.name | string | `nil` | name of the secret containing connection and authentication information |
| externalKeyValueStore.existingSecret.passwordKey | string | `nil` | The password can only be provided via secret. |
| externalKeyValueStore.existingSecret.portKey | string | `nil` | key containing the port for the connection |
| externalKeyValueStore.existingSecret.urlKey | string | `nil` | When set, all other keys and clear values are ignored. |
| externalKeyValueStore.existingSecret.usernameKey | string | `nil` | key containing the username for authentication |
| externalKeyValueStore.host | string | `nil` | host for the connection; alternatively provide via existingSecret.hostKey |
| externalKeyValueStore.port | int | `6379` | port for the connection; alternatively provide via existingSecret.portKey |
| externalKeyValueStore.useTls | bool | `false` | whether the connection should be made with TLS encryption enabled (rediss://) |
| externalKeyValueStore.username | string | `nil` | Omit to authenticate as the default user. |
| extraEnv | list | `[]` | extra environment variables for the operator container |
| extraManifests | list | `[]` | list of extra manifests |
| extraVolumeMounts | list | `[]` | extra volume mounts to add to the operator container |
| extraVolumes | list | `[]` | extra volumes to add to the operator pod (e.g. for CA certificates) |
| fullnameOverride | string | `""` | override the fullname of created resources |
| image.imagePullSecrets | list | `[]` | imagePullSecrets for the operator pod |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.registry | string | `"ghcr.io"` |  |
| image.repository | string | `"mogenius/renovate-operator"` |  |
| image.tag | string | `nil` | tag of the renovate-operator, if empty .Chart.AppVersion is used |
| ingress.annotations | object | `{}` | annotations to place on the ingress |
| ingress.enabled | bool | `false` | whether to enable the ingress renovate-operator |
| ingress.host | string | `nil` | the host the ingress should listen for |
| ingress.ingressClassName | string | `"nginx"` | the ingressclassname for the ingress, default: nginx |
| ingress.labels | object | `{}` | labels to place on the ingress |
| ingress.tls | list | `[]` | tls configuration to be applied on the ingress |
| logging.format | string | `"json"` | log format for the operator, one of: json, console |
| logging.level | string | `"info"` | log level for the operator, one of 'debug', 'info', 'error', 'panic'or any integer value > 0 |
| metrics.dashboard.annotations | object | `{}` |  |
| metrics.dashboard.enabled | bool | `false` | whether to create a Grafana dashboard for the operator metrics |
| metrics.dashboard.grafanaDashboard | object | `{"allowCrossNamespaceImport":false,"instanceSelector":{"matchLabels":{}}}` | all fields below are passed to the GrafanaDashboard CR spec |
| metrics.dashboard.labels.grafana_dashboard | string | `"1"` |  |
| metrics.dashboard.namespace | string | `nil` | namespace to create the dashboard in, defaults to the release namespace |
| metrics.enabled | bool | `false` |  |
| metrics.serviceMonitor.enabled | bool | `false` | whether to create a ServiceMonitor for the operator metrics |
| metrics.serviceMonitor.interval | string | `nil` | scrape interval for the ServiceMonitor, defaults to Prometheus global scrape interval |
| metrics.serviceMonitor.labels | object | `{}` |  |
| nodeSelector | object | `{}` | nodeSelector for the operator pod |
| policy.allowRootUser | bool | `false` | itself and cannot be re-enabled here. |
| policy.allowedHosts | list | `["api.github.com","github.com","gitlab.com","api.bitbucket.org","bitbucket.org","gitea.com","codeberg.org"]` | Set to [] to refuse every destination other than that webhook host. |
| policy.allowedImages | list | `["renovate/renovate","docker.io/renovate/renovate","ghcr.io/renovatebot/renovate"]` | Set to [] to deny every image. |
| policy.allowedServiceAccounts | list | `[]` | Empty allows only the namespace default, i.e. jobs that leave the field unset. |
| policy.enabled | bool | `false` | API server regardless of this setting. |
| policy.requireSecretRefOptIn | bool | `true` | key names, so it is not an arbitrary-key reference. |
| priorityClassName | string | `""` | priorityClassName for the operator pod |
| rbac.ownNamespaceOnly | bool | `false` | support is disabled (no cluster-scoped RBAC, no watch). |
| rbac.watchNamespace | string | `""` | optional: specify a custom namespace to watch (defaults to the release namespace when ownNamespaceOnly is true) |
| replicaCount | int | `1` | number of operator replicas. Leader election is enabled automatically when > 1 |
| resources | object | `{}` | resources for the operator pod |
| route | object | `{"additionalRules":[],"annotations":{},"enabled":false,"group":"","hostnames":[],"kind":"Service","labels":{},"listenerset":{"allowedRoutes":{"namespaces":{"from":"Same"}},"annotations":{},"enabled":false,"hostname":"","labels":{},"listenerName":"https","listeners":[],"parentRef":{},"port":443,"protocol":"HTTPS","tls":{"enabled":false,"mode":"Terminate","secretName":""}},"parentRefs":[],"weight":1}` | HTTPRoute for the operator web ui |
| route.additionalRules | list | `[]` | Any custom rules you want to specify |
| route.annotations | object | `{}` | annotations to place on the HTTPRoute |
| route.enabled | bool | `false` | whether to enable the HTTPRoute |
| route.group | string | `""` | group to place on the HTTPRoute |
| route.hostnames | list | `[]` | hostnames to place on the HTTPRoute |
| route.kind | string | `"Service"` | kind to place on the HTTPRoute |
| route.labels | object | `{}` | labels to place on the HTTPRoute |
| route.listenerset | object | `{"allowedRoutes":{"namespaces":{"from":"Same"}},"annotations":{},"enabled":false,"hostname":"","labels":{},"listenerName":"https","listeners":[],"parentRef":{},"port":443,"protocol":"HTTPS","tls":{"enabled":false,"mode":"Terminate","secretName":""}}` | ListenerSet for the operator web UI (Gateway API v1.3+) |
| route.listenerset.allowedRoutes | object | `{"namespaces":{"from":"Same"}}` | allowedRoutes configuration for the listener |
| route.listenerset.annotations | object | `{}` | annotations to place on the ListenerSet |
| route.listenerset.enabled | bool | `false` | whether to enable the ListenerSet |
| route.listenerset.hostname | string | `""` | hostname for the listener; defaults to the first entry in route.hostnames |
| route.listenerset.labels | object | `{}` | labels to place on the ListenerSet |
| route.listenerset.listenerName | string | `"https"` | name of the listener entry |
| route.listenerset.listeners | list | `[]` | optional: fully custom listeners list; overrides all listener defaults above |
| route.listenerset.parentRef | object | `{}` | parentRef pointing to the parent Gateway (required when enabled) |
| route.listenerset.port | int | `443` | port the listener should bind to |
| route.listenerset.protocol | string | `"HTTPS"` | protocol for the listener (HTTP, HTTPS, TLS); used to derive the auth-redirect URL scheme |
| route.listenerset.tls.enabled | bool | `false` | whether to configure TLS on the listener |
| route.listenerset.tls.mode | string | `"Terminate"` | TLS termination mode (Terminate or Passthrough) |
| route.listenerset.tls.secretName | string | `""` | name of the TLS secret; defaults to <fullname>-tls |
| route.parentRefs | list | `[]` | parentRefs to place on the HTTPRoute |
| route.weight | int | `1` | weight to place on the HTTPRoute |
| s3.bucket | string | `""` | S3 bucket name; required when config.logStorage.mode=s3 or config.forwardS3CacheToJobs=true |
| s3.cachePrefix | string | `"renovate-cache"` | Object key prefix forwarded to Renovate jobs as RENOVATE_S3_URL |
| s3.endpoint | string | `""` | Custom S3-compatible endpoint URL for self-hosted object stores (e.g. RustFS); leave empty for AWS S3 |
| s3.existingSecret | string | `""` | Name of an existing Secret containing S3 credentials (optional; uses the pod's IAM role when absent) The Secret must have keys: access-key-id and secret-access-key |
| s3.forcePathStyle | bool | `false` | Enable path-style S3 addressing (required by some S3-compatible stores); leave false for AWS S3 |
| s3.logPrefix | string | `"renovate-logs"` | Object key prefix used by the operator log store (LOG_STORE_MODE=s3) |
| s3.region | string | `"us-east-1"` | AWS region (defaults to us-east-1) |
| securityContext | object | `{"container":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"privileged":false,"readOnlyRootFilesystem":true,"runAsGroup":1000,"runAsNonRoot":true,"runAsUser":1000,"seccompProfile":{"type":"RuntimeDefault"}},"pod":{"fsGroup":1000,"runAsGroup":1000,"runAsNonRoot":true,"runAsUser":1000,"seccompProfile":{"type":"RuntimeDefault"}}}` | security context for the pod and containers |
| serviceAccount.annotations | object | `{}` | annotations to add to the serviceaccount |
| serviceAccount.create | bool | `true` | whether a serviceaccount should be created |
| serviceAccount.labels | object | `{}` | labels to add to the serviceaccount |
| serviceAccount.name | string | `nil` | name of the serviceaccount, if not set a name is generated using the fullname template |
| telemetry | object | `{"enabled":false,"endpoint":"","forwardToJobs":false,"jobEndpoint":"","protocol":"grpc"}` | OpenTelemetry configuration for the operator |
| telemetry.enabled | bool | `false` | enable OTLP export of traces, metrics, and logs from the operator |
| telemetry.endpoint | string | `""` | OTLP gRPC endpoint for the operator (host:port format, e.g. "my-collector:4317") |
| telemetry.forwardToJobs | bool | `false` | forward OTEL_* env vars to Renovate Jobs (uses Renovate's native OTLP/HTTP exporter) |
| telemetry.jobEndpoint | string | `""` | gRPC host:port so that Jobs receive a proper HTTP URL for Renovate's OTLP/HTTP exporter. |
| telemetry.protocol | string | `"grpc"` | OTLP protocol for the operator. Only "grpc" is currently supported. |
| tolerations | list | `[]` | tolerations for the operator pod |
| topologySpreadConstraints | list | `[]` | topologySpreadConstraints for the operator pod |
| valkey | object | `{"auth":{"aclUsers":{"default":{"password":"","passwordKey":"","permissions":"~* &* +@all"}},"enabled":true,"usersExistingSecret":""},"dataStorage":{"enabled":false},"enabled":false,"existingSecret":"","existingSecretKey":"valkey-url","metrics":{"enabled":false,"serviceMonitor":{"enabled":false}}}` | optional: deploy a Valkey instance for session storage (recommended for multi-replica) |
| valkey.existingSecret | string | `""` | When set, sessions are stored in Valkey (recommended for multi-replica deployments) |
| valkey.existingSecretKey | string | `"valkey-url"` | key in the existing secret that contains the Valkey URL |
| webhook.baseUrl | string | `""` | optional: override the webhook base URL (WEBHOOK_BASE_URL), e.g. when the ingress or gateway is behind a proxy. |
| webhook.baseUrlScheme | string | `""` | optional: override the scheme (http or https) of the auto-derived webhook base URL (WEBHOOK_BASE_URL), e.g. when TLS terminates at an external load balancer or Gateway |
| webhook.enabled | bool | `false` |  |
| webhook.ingress.annotations | object | `{}` | annotations to place on the ingress |
| webhook.ingress.enabled | bool | `false` | whether to enable the ingress for the webhook server |
| webhook.ingress.host | string | `nil` | the host the ingress should lsiten for |
| webhook.ingress.ingressClassName | string | `"nginx"` | the ingressclassname for the ingress, default: nginx |
| webhook.ingress.labels | object | `{}` | labels to place on the ingress |
| webhook.ingress.tls | list | `[]` | tls configuration to be applied on the ingress |
| webhook.port | int | `8082` |  |
| webhook.route | object | `{"additionalRules":[],"annotations":{},"enabled":false,"group":"","hostnames":[],"kind":"Service","labels":{},"listenerset":{"allowedRoutes":{"namespaces":{"from":"Same"}},"annotations":{},"enabled":false,"hostname":"","labels":{},"listenerName":"https","listeners":[],"parentRef":{},"port":443,"protocol":"HTTPS","tls":{"enabled":false,"mode":"Terminate","secretName":""}},"parentRefs":[],"weight":1}` | HTTPRoute for the webhook server |
| webhook.route.additionalRules | list | `[]` | Any custom rules you want to specify |
| webhook.route.annotations | object | `{}` | annotations to place on the HTTPRoute |
| webhook.route.enabled | bool | `false` | whether to enable the HTTPRoute |
| webhook.route.group | string | `""` | group to place on the HTTPRoute |
| webhook.route.hostnames | list | `[]` | hostnames to place on the HTTPRoute |
| webhook.route.kind | string | `"Service"` | kind to place on the HTTPRoute |
| webhook.route.labels | object | `{}` | labels to place on the HTTPRoute |
| webhook.route.listenerset | object | `{"allowedRoutes":{"namespaces":{"from":"Same"}},"annotations":{},"enabled":false,"hostname":"","labels":{},"listenerName":"https","listeners":[],"parentRef":{},"port":443,"protocol":"HTTPS","tls":{"enabled":false,"mode":"Terminate","secretName":""}}` | ListenerSet for the webhook server (Gateway API v1.3+) |
| webhook.route.listenerset.allowedRoutes | object | `{"namespaces":{"from":"Same"}}` | allowedRoutes configuration for the listener |
| webhook.route.listenerset.annotations | object | `{}` | annotations to place on the ListenerSet |
| webhook.route.listenerset.enabled | bool | `false` | whether to enable the ListenerSet |
| webhook.route.listenerset.hostname | string | `""` | hostname for the listener; defaults to the first entry in webhook.route.hostnames |
| webhook.route.listenerset.labels | object | `{}` | labels to place on the ListenerSet |
| webhook.route.listenerset.listenerName | string | `"https"` | name of the listener entry |
| webhook.route.listenerset.listeners | list | `[]` | optional: fully custom listeners list; overrides all listener defaults above |
| webhook.route.listenerset.parentRef | object | `{}` | parentRef pointing to the parent Gateway (required when enabled) |
| webhook.route.listenerset.port | int | `443` | port the listener should bind to |
| webhook.route.listenerset.protocol | string | `"HTTPS"` | protocol for the listener (HTTP, HTTPS, TLS) |
| webhook.route.listenerset.tls.enabled | bool | `false` | whether to configure TLS on the listener |
| webhook.route.listenerset.tls.mode | string | `"Terminate"` | TLS termination mode (Terminate or Passthrough) |
| webhook.route.listenerset.tls.secretName | string | `""` | name of the TLS secret; defaults to <fullname>-webhook-tls |
| webhook.route.parentRefs | list | `[]` | parentRefs to place on the HTTPRoute |
| webhook.route.weight | int | `1` | weight to place on the HTTPRoute |
| webhook.unifiedWebhookHost | bool | `false` | if set to true the ui host will be used for webhooks |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
