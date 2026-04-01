package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/clientProvider"
	"renovate-operator/config"
	"renovate-operator/controllers"
	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	"renovate-operator/health"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
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
			Key:      "DEFAULT_ALLOWED_GROUPS",
			Optional: true,
			Default:  "",
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
			Key:      "VALKEY_PASSWORD",
			Optional: true,
			Default:  "",
		},
	})
	assert.NoError(err, "failed to initialize config module")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	cfg := ctrl.GetConfigOrDie()
	adaptKubeConfig(cfg)

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	metricStore.Register(ctrlmetrics.Registry)

	watchNamespace := config.GetValue("WATCH_NAMESPACE")
	leaderElectionID := config.GetValue("LEADER_ELECTION_ID")
	mgrOptions := ctrl.Options{
		Scheme:                        nil,
		LeaderElection:                leaderElectionID != "",
		LeaderElectionID:              leaderElectionID,
		LeaderElectionNamespace:       config.GetValue("POD_NAMESPACE"),
		LeaderElectionReleaseOnCancel: true,
		Cache:                         cache.Options{DefaultNamespaces: map[string]cache.Config{watchNamespace: {}}},
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

	gitProviderClientFactory := gitProviderClientFactory.NewGitProviderClientFactory(mgr.GetClient())

	jobMgr := crdManager.NewRenovateJobManager(mgr.GetClient(), gitProviderClientFactory, ctrl.Log.WithName("job-manager"))

	discovery := renovate.NewDiscoveryAgent(
		mgr.GetScheme(),
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-discovery"),
	)

	cronManager := scheduler.NewScheduler(ctrl.Log.WithName("scheduler"), health)

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

	// Initialize session store (Valkey if configured, otherwise in-memory)
	sessionStore, storeType, storeErr := ui.NewSessionStore(ui.ValkeyConfig{
		URL:      config.GetValue("VALKEY_URL"),
		Host:     config.GetValue("VALKEY_HOST"),
		Port:     config.GetValue("VALKEY_PORT"),
		Password: config.GetValue("VALKEY_PASSWORD"),
	}, storeKey)
	assert.NoError(storeErr, "failed to initialize session store")
	ctrl.Log.WithName("auth").Info("Using session store", "type", storeType)
	if storeType == "memory" && leaderElectionID != "" {
		ctrl.Log.WithName("auth").Info("WARNING: in-memory session store with multiple replicas will cause sessions to break across pods; configure Valkey for multi-replica deployments")
	}
	defer func() {
		if err := sessionStore.Close(); err != nil {
			ctrl.Log.WithName("auth").Error(err, "failed to close session store")
		}
	}()

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
			LogoutURL:           config.GetValue("OIDC_LOGOUT_URL"),
			AllowedGroupPrefix:  config.GetValue("OIDC_ALLOWED_GROUP_PREFIX"),
			AllowedGroupPattern: config.GetValue("OIDC_ALLOWED_GROUP_PATTERN"),
			AdditionalScopes:    splitAndTrim(config.GetValue("OIDC_ADDITIONAL_SCOPES"), ","),
			FetchUserInfoGroups: config.GetValue("OIDC_FETCH_USERINFO_GROUPS") == "true",
		}, cookieKey, ctrl.Log.WithName("oidc"), sessionStore)
		assert.NoError(oidcErr, "failed to initialize OIDC provider")
		authProvider = oidcAuth
		ctrl.Log.WithName("auth").Info("OIDC authentication enabled", "issuer", oidcIssuer)

		// Log group filtering configuration
		if config.GetValue("OIDC_ALLOWED_GROUP_PREFIX") != "" {
			ctrl.Log.WithName("auth").Info("OIDC group prefix filter enabled",
				"prefix", config.GetValue("OIDC_ALLOWED_GROUP_PREFIX"))
		}
		if config.GetValue("OIDC_ALLOWED_GROUP_PATTERN") != "" {
			ctrl.Log.WithName("auth").Info("OIDC group pattern filter enabled",
				"pattern", config.GetValue("OIDC_ALLOWED_GROUP_PATTERN"))
		}
	} else if githubClientID != "" && githubClientSecret != "" {
		ghAuth, ghErr := ui.NewGitHubOAuth(ui.GitHubOAuthConfig{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			RedirectURL:  config.GetValue("GITHUB_REDIRECT_URL"),
		}, cookieKey, ctrl.Log.WithName("github-oauth"), sessionStore)
		assert.NoError(ghErr, "failed to initialize GitHub OAuth provider")
		authProvider = ghAuth
		ctrl.Log.WithName("auth").Info("GitHub OAuth authentication enabled")
	} else {
		ctrl.Log.WithName("auth").Info("No authentication configured, UI access is unauthenticated")
	}

	// Parse default allowed groups (comma-separated)
	var defaultAllowedGroups []string
	defaultGroupsStr := config.GetValue("DEFAULT_ALLOWED_GROUPS")
	if defaultGroupsStr != "" {
		for _, group := range splitAndTrim(defaultGroupsStr, ",") {
			if group != "" {
				normalized := strings.ToLower(strings.TrimSpace(group))
				defaultAllowedGroups = append(defaultAllowedGroups, normalized)
			}
		}
		ctrl.Log.WithName("auth").Info("Default allowed groups configured", "groups", defaultAllowedGroups)
	}

	if authProvider != nil && !authProvider.SupportsGroups() && len(defaultAllowedGroups) > 0 {
		ctrl.Log.WithName("auth").Error(nil,
			"auth provider does not support group claims -- DEFAULT_ALLOWED_GROUPS is configured but will have no effect, all jobs will be hidden from all users",
			"ignored_groups", defaultAllowedGroups)
	}

	// UI and webhook servers run on all replicas
	uiServer := ui.NewServer(jobMgr, discovery, cronManager, ctrl.Log.WithName("ui-server"), health, Version, authProvider, defaultAllowedGroups)
	uiServer.Run()

	if config.GetValue("WEBHOOK_SERVER_ENABLED") != "false" {
		webhookServer := webhook.NewWebookServer(jobMgr, ctrl.Log.WithName("webhook"))
		webhookServer.Run()
	}

	executor := renovate.NewRenovateExecutor(
		mgr.GetScheme(),
		jobMgr,
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-executor"),
		health,
	)

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

	err = (&controllers.RenovateJobReconciler{
		Scheduler: cronManager,
		Manager:   jobMgr,
		Discovery: discovery,
		K8sClient: mgr.GetClient(),
	}).SetupWithManager(mgr)
	assert.NoError(err, "failed to setup manager")

	err = mgr.Start(ctx)
	assert.NoError(err, "failed to start manager")
}
