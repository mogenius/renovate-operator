package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/clientProvider"
	"renovate-operator/config"
	"renovate-operator/controllers"
	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	"renovate-operator/github"
	"renovate-operator/health"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/objectstore"
	"renovate-operator/internal/podLogs"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/metricStore"
	"renovate-operator/scheduler"
	"renovate-operator/ui"
	"renovate-operator/webhook"

	"k8s.io/client-go/rest"

	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var Version = "dev" // default version, will be overridden by ld build flag in Dockerfile

func adaptKubeConfig(cfg *rest.Config) {
	cfg.QPS = 50
	cfg.Burst = 100
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

type authSetup struct {
	provider       ui.AuthProvider
	kvStore        kvstore.KVStore
	accessDefaults ui.AccessDefaults
	cleanup        func()
}

func initAuth(valkeyConf kvstore.ValkeyConfig) authSetup {
	log := ctrl.Log.WithName("auth")

	// Determine the session secret for the active auth provider so we can
	// derive the encryption key early (needed for both the Valkey store and
	// the auth provider itself).
	oidcIssuer := config.GetValue("OIDC_ISSUER_URL")
	oidcClientID := config.GetValue("OIDC_CLIENT_ID")
	oidcClientSecret := config.GetValue("OIDC_CLIENT_SECRET")
	githubClientID := config.GetValue("GITHUB_CLIENT_ID")
	githubClientSecret := config.GetValue("GITHUB_CLIENT_SECRET")

	var sessionSecret string
	if oidcIssuer != "" && oidcClientID != "" && oidcClientSecret != "" {
		sessionSecret = config.GetValue("OIDC_SESSION_SECRET")
	} else if githubClientID != "" && githubClientSecret != "" {
		sessionSecret = config.GetValue("GITHUB_SESSION_SECRET")
	}

	encryptionKey, encKeyErr := ui.ComputeEncryptionKey(sessionSecret)
	assert.NoError(encKeyErr, "failed to compute session encryption key")

	// Derive separate keys for cookie encryption and session store at-rest
	// encryption so a compromise of one does not affect the other.
	cookieKey, storeKey := ui.DeriveSubKeys(encryptionKey)

	// Initialize KV store (Valkey if configured, otherwise nil)
	kvStore, kvErr := kvstore.NewKVStore(valkeyConf, kvstore.UsageSessionStore)
	if kvErr != nil && kvErr != kvstore.ErrValkeyNotConfigured {
		assert.NoError(kvErr, "failed to initialize KV store")
	}

	// Wrap KV store with session-specific encryption and key prefix
	sessionStore, storeErr := ui.NewSessionStore(kvStore, storeKey)
	assert.NoError(storeErr, "failed to initialize session store")

	if kvStore != nil {
		log.Info("Using session store", "type", "valkey")
	} else {
		log.Info("Using session store", "type", "cookie")
		log.Info("Cookie-based sessions: session data is stored in the browser cookie. If users belong to many groups, the cookie may exceed browser size limits; configure Valkey to avoid this. For multi-replica deployments, Valkey is recommended for session sharing across pods.")
	}

	cleanup := func() {
		if kvStore != nil {
			if err := kvStore.Close(); err != nil {
				log.Error(err, "failed to close KV store")
			}
		}
	}

	// Initialize authentication provider (OIDC or GitHub OAuth)
	var authProvider ui.AuthProvider

	if oidcIssuer != "" && oidcClientID != "" && oidcClientSecret != "" {
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer oidcCancel()
		oidcAuth, oidcErr := ui.NewOIDCAuth(oidcCtx, ui.OIDCConfig{
			IssuerURL:           oidcIssuer,
			ClientID:            oidcClientID,
			ClientSecret:        oidcClientSecret,
			RedirectURL:         config.GetValue("OIDC_REDIRECT_URL"),
			InsecureSkipVerify:  config.GetValue("OIDC_INSECURE_SKIP_VERIFY") == "true",
			CACertPath:          config.GetValue("OIDC_CA_CERT_PATH"),
			LogoutURL:           config.GetValue("OIDC_LOGOUT_URL"),
			AllowedGroupPrefix:  config.GetValue("OIDC_ALLOWED_GROUP_PREFIX"),
			AllowedGroupPattern: config.GetValue("OIDC_ALLOWED_GROUP_PATTERN"),
			AdditionalScopes:    splitAndTrim(config.GetValue("OIDC_ADDITIONAL_SCOPES"), ","),
			FetchUserInfoGroups: config.GetValue("OIDC_FETCH_USERINFO_GROUPS") == "true",
			PKCEEnabled:         config.GetValue("OIDC_PKCE_ENABLED") != "false",
			GroupsClaim:         config.GetValue("OIDC_GROUPS_CLAIM"),
		}, cookieKey, ctrl.Log.WithName("oidc"), sessionStore)
		assert.NoError(oidcErr, "failed to initialize OIDC provider")
		authProvider = oidcAuth
		log.Info("OIDC authentication enabled", "issuer", oidcIssuer)

		// Log group filtering configuration
		if config.GetValue("OIDC_ALLOWED_GROUP_PREFIX") != "" {
			log.Info("OIDC group prefix filter enabled",
				"prefix", config.GetValue("OIDC_ALLOWED_GROUP_PREFIX"))
		}
		if config.GetValue("OIDC_ALLOWED_GROUP_PATTERN") != "" {
			log.Info("OIDC group pattern filter enabled",
				"pattern", config.GetValue("OIDC_ALLOWED_GROUP_PATTERN"))
		}
	} else if githubClientID != "" && githubClientSecret != "" {
		ghAuth, ghErr := ui.NewGitHubOAuth(ui.GitHubOAuthConfig{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			RedirectURL:  config.GetValue("GITHUB_REDIRECT_URL"),
			OrgGroups:    config.GetValue("GITHUB_ORG_GROUPS") == "true",
		}, cookieKey, ctrl.Log.WithName("github-oauth"), sessionStore)
		assert.NoError(ghErr, "failed to initialize GitHub OAuth provider")
		authProvider = ghAuth
		log.Info("GitHub OAuth authentication enabled", "orgGroups", config.GetValue("GITHUB_ORG_GROUPS") == "true")
	} else {
		log.Info("No authentication configured, UI access is unauthenticated")
	}

	accessDefaults := parseAccessDefaults(log)

	return authSetup{
		provider:       authProvider,
		kvStore:        kvStore,
		accessDefaults: accessDefaults,
		cleanup:        cleanup,
	}
}

// parseAccessDefaults reads the operator-wide access defaults that apply to
// RenovateJobs which leave parts of their access configuration unset.
func parseAccessDefaults(log logr.Logger) ui.AccessDefaults {
	defaults := ui.AccessDefaults{
		ReaderGroups:      parseGroupList(config.GetValue("DEFAULT_READER_GROUPS")),
		AdminGroups:       parseGroupList(config.GetValue("DEFAULT_ADMIN_GROUPS")),
		AnonymousRead:     config.GetValue("DEFAULT_ANONYMOUS_READ") == "true",
		AnonymousReadLogs: config.GetValue("DEFAULT_ANONYMOUS_READ_LOGS") == "true",
	}

	// The deprecated DEFAULT_ALLOWED_GROUPS granted what is now admin access.
	if legacy := parseGroupList(config.GetValue("DEFAULT_ALLOWED_GROUPS")); len(legacy) > 0 {
		log.Info("DEFAULT_ALLOWED_GROUPS is deprecated, use DEFAULT_ADMIN_GROUPS (auth.defaultAccess.adminGroups)",
			"groups", legacy)
		defaults.AdminGroups = append(defaults.AdminGroups, legacy...)
	}

	log.Info("Default access configured",
		"readerGroups", defaults.ReaderGroups,
		"adminGroups", defaults.AdminGroups,
		"anonymousRead", defaults.AnonymousRead,
		"anonymousReadLogs", defaults.AnonymousReadLogs)

	return defaults
}

// parseGroupList splits a comma-separated group list and normalizes it the same
// way session groups are normalized, so both sides of a comparison match.
func parseGroupList(value string) []string {
	if value == "" {
		return nil
	}
	groups := splitAndTrim(value, ",")
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		normalized = append(normalized, strings.ToLower(group))
	}
	return normalized
}

// assertAccessRulesEnforceable refuses to start when access rules are configured
// but the auth provider cannot supply the groups they are written against.
// Without this the operator would boot and silently hide every job.
//
// Jobs created after this check are not covered: they fail closed and log an
// error when the UI resolves them.
func assertAccessRulesEnforceable(ctx context.Context, log logr.Logger, provider ui.AuthProvider, defaults ui.AccessDefaults, cfg *rest.Config, scheme *runtime.Scheme) {
	if provider == nil || provider.SupportsGroups() {
		return
	}

	if len(defaults.ReaderGroups) > 0 || len(defaults.AdminGroups) > 0 {
		assert.Assert(false,
			"auth provider cannot supply groups but default access groups are configured. "+
				"Set auth.github.orgGroups=true (GITHUB_ORG_GROUPS) or remove DEFAULT_READER_GROUPS, "+
				"DEFAULT_ADMIN_GROUPS and DEFAULT_ALLOWED_GROUPS")
	}

	// The manager's cache is not running yet, so this one-shot read goes
	// straight to the API server.
	directClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create client to validate access rules, continuing")
		return
	}

	var jobList api.RenovateJobList
	if err := directClient.List(ctx, &jobList); err != nil {
		log.Error(err, "failed to list RenovateJobs to validate access rules, continuing")
		return
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		hasGroups := len(job.Spec.AllowedGroups) > 0 //nolint:staticcheck // deprecated field is intentionally still honoured
		if job.Spec.Access != nil {
			hasGroups = hasGroups || len(job.Spec.Access.ReaderGroups) > 0 || len(job.Spec.Access.AdminGroups) > 0
		}
		assert.Assert(!hasGroups, fmt.Sprintf(
			"RenovateJob %s/%s configures access groups but the auth provider cannot supply groups. "+
				"Set auth.github.orgGroups=true (GITHUB_ORG_GROUPS) or remove the group configuration",
			job.Namespace, job.Name))
	}
}

func main() {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{
			Key:      "SERVER_PORT",
			Optional: true,
			Default:  "8081",
			Validate: func(value string) error {
				_, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("'SERVER_PORT' needs to be an integer: %s", err.Error())
				}
				return nil
			},
		},
		{
			Key:      "WEBHOOK_SERVER_PORT",
			Optional: true,
			Default:  "8082",
			Validate: func(value string) error {
				_, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("'WEBHOOK_SERVER_PORT' needs to be an integer: %s", err.Error())
				}
				return nil
			},
		},
		{
			Key:      "WEBHOOK_SERVER_ENABLED",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "BASE_PATH",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "WEBHOOK_BASE_URL",
			Optional: true,
		},
		{
			Key:      "WEBHOOK_SERVER_UNIFIED_HOST",
			Optional: true,
			Default:  "false",
			Validate: func(value string) error {
				if value != "true" && value != "false" {
					return fmt.Errorf("'WEBHOOK_SERVER_UNIFIED_HOST' must be 'true' or 'false'")
				}
				return nil
			},
		},
		{
			Key:      "DELETE_SUCCESSFUL_JOBS",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "JOB_TIMEOUT_SECONDS",
			Optional: true,
			Default:  "1800",
			Validate: func(value string) error {
				_, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return fmt.Errorf("'JOB_TIMEOUT_SECONDS' needs to be an integer: %s", err.Error())
				}
				return nil
			},
		},
		{
			Key:      "JOB_BACKOFF_LIMIT",
			Optional: true,
			Default:  "1",
			Validate: func(value string) error {
				_, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return fmt.Errorf("'JOB_BACKOFF_LIMIT' needs to be an integer: %s", err.Error())
				}
				return nil
			},
		},
		{
			Key:      "JOB_TTL_SECONDS_AFTER_FINISHED",
			Optional: true,
			Default:  "-1",
			Validate: func(value string) error {
				parsed, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return fmt.Errorf("'JOB_TTL_SECONDS_AFTER_FINISHED' needs to be an integer: %s", err.Error())
				}
				if parsed < -1 {
					return fmt.Errorf("'JOB_TTL_SECONDS_AFTER_FINISHED' needs to be -1 or greater")
				}
				return nil
			},
		},
		{
			Key:      "WATCH_NAMESPACE",
			Optional: true,
		},
		{
			Key:      "POD_NAMESPACE",
			Optional: true,
		},
		{
			Key:      "IMAGE_PULL_SECRETS",
			Optional: true,
			Default:  "[]",
		},
		{
			Key:      "LEADER_ELECTION_ID",
			Optional: true,
		},
		{
			Key:      "OIDC_ISSUER_URL",
			Optional: true,
		},
		{
			Key:      "OIDC_CLIENT_ID",
			Optional: true,
		},
		{
			Key:      "OIDC_CLIENT_SECRET",
			Optional: true,
		},
		{
			Key:      "OIDC_REDIRECT_URL",
			Optional: true,
		},
		{
			Key:      "OIDC_SESSION_SECRET",
			Optional: true,
		},
		{
			Key:      "OIDC_INSECURE_SKIP_VERIFY",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "OIDC_CA_CERT_PATH",
			Optional: true,
		},
		{
			Key:      "OIDC_LOGOUT_URL",
			Optional: true,
		},
		{
			Key:      "GITHUB_CLIENT_ID",
			Optional: true,
		},
		{
			Key:      "GITHUB_CLIENT_SECRET",
			Optional: true,
		},
		{
			Key:      "GITHUB_REDIRECT_URL",
			Optional: true,
		},
		{
			Key:      "GITHUB_SESSION_SECRET",
			Optional: true,
		},
		{
			Key:      "GITHUB_ORG_GROUPS",
			Optional: true,
			Default:  "false",
		},
		{
			// Deprecated: use DEFAULT_ADMIN_GROUPS
			Key:      "DEFAULT_ALLOWED_GROUPS",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "DEFAULT_READER_GROUPS",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "DEFAULT_ADMIN_GROUPS",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "DEFAULT_ANONYMOUS_READ",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "DEFAULT_ANONYMOUS_READ_LOGS",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "OIDC_ALLOWED_GROUP_PREFIX",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "OIDC_ALLOWED_GROUP_PATTERN",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "OIDC_ADDITIONAL_SCOPES",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "OIDC_FETCH_USERINFO_GROUPS",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "OIDC_PKCE_ENABLED",
			Optional: true,
			Default:  "true",
		},
		{
			Key:      "OIDC_GROUPS_CLAIM",
			Optional: true,
			Default:  "groups",
		},
		{
			Key:      "VALKEY_URL",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "VALKEY_HOST",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "VALKEY_PORT",
			Optional: true,
			Default:  "6379",
		},
		{
			Key:      "VALKEY_USERNAME",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "VALKEY_PASSWORD",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "VALKEY_TLS",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "VALKEY_FORWARD_CACHE_TO_JOBS",
			Optional: true,
			Default:  "true",
		},
		{
			Key:      "LOG_STORE_MODE",
			Optional: true,
			Default:  "disabled",
			Validate: func(value string) error {
				switch value {
				case "disabled", "memory", "valkey", "s3":
					return nil
				}
				return fmt.Errorf("'LOG_STORE_MODE' must be one of: disabled, memory, valkey, s3")
			},
		},
		{
			Key:      "S3_BUCKET",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "S3_REGION",
			Optional: true,
			Default:  "us-east-1",
		},
		{
			Key:      "S3_ENDPOINT",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "S3_ACCESS_KEY_ID",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "S3_SECRET_ACCESS_KEY",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "S3_LOG_PREFIX",
			Optional: true,
			Default:  "renovate-logs",
		},
		{
			Key:      "S3_CACHE_PREFIX",
			Optional: true,
			Default:  "renovate-cache",
		},
		{
			Key:      "S3_FORWARD_CACHE_TO_JOBS",
			Optional: true,
			Default:  "true",
		},
		{
			Key:      "S3_FORCE_PATH_STYLE",
			Optional: true,
			Default:  "false",
		},
		{
			Key:      "S3_CREDENTIALS_SECRET_NAME",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "GLOBAL_PARALLELISM_LIMIT",
			Optional: true,
			Default:  "0",
			Validate: func(value string) error {
				parsed, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("'GLOBAL_PARALLELISM_LIMIT' needs to be an integer: %s", err.Error())
				}
				if parsed < 0 {
					return fmt.Errorf("'GLOBAL_PARALLELISM_LIMIT' must be 0 (unlimited) or a positive integer")
				}
				return nil
			},
		},
		{
			Key:      "POD_LABEL_TEMPLATES",
			Optional: true,
			Default:  "{}",
			Validate: func(value string) error {
				var templates map[string]string
				if err := json.Unmarshal([]byte(value), &templates); err != nil {
					return fmt.Errorf("'POD_LABEL_TEMPLATES' must be a JSON object of label-key to template string: %w", err)
				}
				return nil
			},
		},
		{
			Key:      "POLICY_ENABLED",
			Optional: true,
			Default:  "false",
			Validate: func(value string) error {
				if value != "true" && value != "false" {
					return fmt.Errorf("'POLICY_ENABLED' must be 'true' or 'false'")
				}
				return nil
			},
		},
		{
			Key:      "POLICY_ALLOWED_HOSTS",
			Optional: true,
			Default:  "",
			Validate: func(value string) error {
				if err := policy.ValidateAllowedHosts(value); err != nil {
					return fmt.Errorf("'POLICY_ALLOWED_HOSTS' %w", err)
				}
				return nil
			},
		},
		{
			Key:      "POLICY_REQUIRE_SECRET_REF_OPT_IN",
			Optional: true,
			Default:  "true",
			Validate: func(value string) error {
				if value != "true" && value != "false" {
					return fmt.Errorf("'POLICY_REQUIRE_SECRET_REF_OPT_IN' must be 'true' or 'false'")
				}
				return nil
			},
		},
		{
			Key:      "POLICY_ALLOWED_SERVICE_ACCOUNTS",
			Optional: true,
			Default:  "",
		},
		{
			Key:      "POLICY_ALLOW_ROOT_USER",
			Optional: true,
			Default:  "false",
			Validate: func(value string) error {
				if value != "true" && value != "false" {
					return fmt.Errorf("'POLICY_ALLOW_ROOT_USER' must be 'true' or 'false'")
				}
				return nil
			},
		},
		{
			Key:      "POLICY_ALLOWED_IMAGES",
			Optional: true,
			Default:  "",
			Validate: func(value string) error {
				if err := policy.ValidateAllowedImages(value); err != nil {
					return fmt.Errorf("'POLICY_ALLOWED_IMAGES' %w", err)
				}
				return nil
			},
		},
	})
	assert.NoError(err, "failed to initialize config module")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	cfg := ctrl.GetConfigOrDie()
	adaptKubeConfig(cfg)

	otelCleanup := initObservability(&opts)
	defer otelCleanup()

	watchNamespace := config.GetValue("WATCH_NAMESPACE")
	leaderElectionID := config.GetValue("LEADER_ELECTION_ID")
	mgrOptions := ctrl.Options{
		Scheme:                        nil,
		LeaderElection:                leaderElectionID != "",
		LeaderElectionID:              leaderElectionID,
		LeaderElectionNamespace:       config.GetValue("POD_NAMESPACE"),
		LeaderElectionReleaseOnCancel: true,
		Cache:                         cache.Options{DefaultNamespaces: map[string]cache.Config{watchNamespace: {}}},
		// Secrets bypass the informer cache: a cached read needs list+watch on every
		// Secret in the watched scope and keeps all of their values in memory, while
		// the operator only ever reads a handful by name.
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&corev1.Secret{}},
			},
		},
	}

	mgr, err := ctrl.NewManager(cfg, mgrOptions)
	assert.NoError(err, "failed to create new manager")

	// Register the RenovateJob types with the scheme
	err = api.AddToScheme(mgr.GetScheme())
	assert.NoError(err, "failed to register scheme")

	err = clientProvider.InitializeStaticClientProvider()
	assert.NoError(err, "failed to create static clientprovider")

	health := health.NewHealthCheck()
	ctx := ctrl.SetupSignalHandler()

	guardRails := policy.FromConfig()
	policyLog := ctrl.Log.WithName("policy")
	metricStore.SetPolicyEnabled(!guardRails.Disabled)
	if guardRails.Disabled {
		// Loud on purpose, and on every start: this is the default, so most operators
		// see it without having chosen it. Anyone able to write a RenovateJob can have
		// the operator read any secret in its namespace and send it, or the job's
		// platform token, to a host of their choosing.
		policyLog.Info("WARNING: ############################################################")
		policyLog.Info("WARNING: the policy engine is NOT ENABLED -- this operator is running in an UNSECURED mode.")
		policyLog.Info("WARNING: any principal that can create or edit a RenovateJob in any watched namespace can:")
		policyLog.Info("WARNING:   * have the operator read any secret in that namespace, at any key, and send it to a host they choose")
		policyLog.Info("WARNING:   * redirect this job's Renovate platform token to a host they choose")
		policyLog.Info("WARNING:   * point your repositories' webhooks at a host they choose, persistently")
		policyLog.Info("WARNING:   * run the Renovate pod as another ServiceAccount, as root, or from any image")
		policyLog.Info("WARNING: enforcement is off by default so a new install works out of the box -- it is meant to be turned on.")
		policyLog.Info("WARNING: turn it on with policy.enabled=true (POLICY_ENABLED) -- configure policy.allowedHosts first.")
		policyLog.Info("WARNING: see docs/migration-v5-to-v6.md")
		policyLog.Info("WARNING: ############################################################")
	} else {
		if len(guardRails.AllowedHosts) == 0 {
			policyLog.Error(nil,
				"no allowed destination hosts configured -- every RenovateJob will be refused, no Renovate run will start and no webhook will be synced. Set policy.allowedHosts (POLICY_ALLOWED_HOSTS) to the platform hosts this operator may talk to, or set policy.enabled=false to turn the policy engine off entirely")
		} else {
			policyLog.Info("Destination policy active", "allowedHosts", guardRails.AllowedHosts)
		}
		if guardRails.AllowUnlabeledSecretRefs {
			policyLog.Info("WARNING: secret reference opt-in is disabled. A RenovateJob may have the operator read any secret key in its namespace. " +
				"Prefer leaving policy.requireSecretRefOptIn enabled and labelling the secrets you intend to reference.")
		}
	}

	gitProviderClientFactory := gitProviderClientFactory.NewGitProviderClientFactory(mgr.GetClient(), guardRails)

	valkeyConf := kvstore.ConfigFromEnv(config.GetValue)

	s3Cfg := objectstore.S3Config{
		Bucket:          config.GetValue("S3_BUCKET"),
		Region:          config.GetValue("S3_REGION"),
		Endpoint:        config.GetValue("S3_ENDPOINT"),
		ForcePathStyle:  config.GetValue("S3_FORCE_PATH_STYLE") == "true",
		AccessKeyID:     config.GetValue("S3_ACCESS_KEY_ID"),
		SecretAccessKey: config.GetValue("S3_SECRET_ACCESS_KEY"),
	}

	ls, err := logStore.NewLogStore(ctrl.Log.WithName("logStore"), config.GetValue("LOG_STORE_MODE"), valkeyConf, s3Cfg, config.GetValue("S3_LOG_PREFIX"))
	assert.NoError(err, "failed to initialize logStore")

	cp := clientProvider.StaticClientProvider()
	clientset, err := cp.K8sClientSet()
	assert.NoError(err, "failed to get Kubernetes clientset for pod log reader")
	podLogReader := podLogs.New(clientset)

	jobMgr := crdManager.NewRenovateJobManager(mgr.GetClient(), gitProviderClientFactory, ctrl.Log.WithName("job-manager"), ls, podLogReader, guardRails)

	discovery := renovate.NewDiscoveryAgent(
		mgr.GetScheme(),
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-discovery"),
		jobMgr,
		podLogReader,
		guardRails,
	)

	cronManager := scheduler.NewScheduler(ctrl.Log.WithName("scheduler"), health)

	auth := initAuth(valkeyConf)
	defer auth.cleanup()

	assertAccessRulesEnforceable(ctx, ctrl.Log.WithName("auth"), auth.provider, auth.accessDefaults, cfg, mgr.GetScheme())

	// UI and webhook servers run on all replicas
	uiServer := ui.NewServer(jobMgr, discovery, cronManager, ctrl.Log.WithName("ui-server"), health, Version, auth.provider, auth.accessDefaults)

	if config.GetValue("WEBHOOK_SERVER_ENABLED") != "false" {
		webhookServer := webhook.NewWebookServer(jobMgr, ctrl.Log.WithName("webhook"))

		if config.GetValue("WEBHOOK_SERVER_UNIFIED_HOST") == "false" {
			webhookServer.Run()
		} else {
			webhook.RegisterWebhookRoutes(uiServer.Router, webhookServer)
		}
	}
	uiServer.Run()

	executor := renovate.NewRenovateExecutor(
		mgr.GetScheme(),
		jobMgr,
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-executor"),
		health,
		ls,
		podLogReader,
		guardRails,
	)

	githubAppToken := github.NewGitHubAppTokenCreatorWithLogger(mgr.GetClient(), ctrl.Log.WithName("github-app-token"), guardRails)

	// Executor and scheduler must only run on the leader to prevent duplicate jobs.
	// When leadership is lost, controller-runtime cancels ctx and the process exits.
	go func() {
		<-mgr.Elected()
		ctrl.Log.WithName("leader-election").Info("this instance is the leader, starting executor and scheduler")
		cronManager.Start()
		if err := executor.Start(ctx); err != nil {
			ctrl.Log.WithName("leader-election").Error(err, "failed to start executor")
		}
	}()

	err = (&controllers.JobReconciler{
		Executor:  executor,
		Discovery: discovery,
		K8sClient: mgr.GetClient(),
	}).SetupWithManager(mgr)
	assert.NoError(err, "failed to setup job manager")

	err = (&controllers.RenovateJobReconciler{
		Scheduler: cronManager,
		Manager:   jobMgr,
		Discovery: discovery,
		K8sClient: mgr.GetClient(),
		GithubApp: githubAppToken,
		Policy:    guardRails,
	}).SetupWithManager(mgr)
	assert.NoError(err, "failed to setup manager")

	err = mgr.Start(ctx)
	assert.NoError(err, "failed to start manager")
}

// initObservability sets up OpenTelemetry (traces, metrics, logs), configures the
// controller-runtime logger with an OTel tee when enabled, and registers Prometheus
// metrics. Returns a cleanup function that flushes OTel providers.
func initObservability(zapOpts *zap.Options) func() {
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()
	otelShutdown, otelEnabled, otelErr := telemetry.SetupOTelSDK(initCtx, Version)

	zapLogger := zap.New(zap.UseFlagOptions(zapOpts))
	if otelEnabled && otelErr == nil {
		otelSink := telemetry.NewOTelLogSink("renovate-operator")
		tee := telemetry.NewTeeLogSink(zapLogger.GetSink(), otelSink)
		zapLogger = logr.New(tee)
	}
	ctrl.SetLogger(zapLogger)

	if otelErr != nil {
		ctrl.Log.WithName("telemetry").Error(otelErr, "failed to initialize OpenTelemetry")
	}

	metricStore.Register(ctrlmetrics.Registry)

	if !otelEnabled || otelErr != nil {
		return func() {}
	}
	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if shutdownErr := otelShutdown(shutdownCtx); shutdownErr != nil {
			ctrl.Log.WithName("telemetry").Error(shutdownErr, "failed to shut down OpenTelemetry")
		}
	}
}
