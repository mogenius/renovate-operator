package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/clientProvider"
	"renovate-operator/config"
	"renovate-operator/controllers"
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

	jobMgr := crdManager.NewRenovateJobManager(mgr.GetClient())

	discovery := renovate.NewDiscoveryAgent(
		mgr.GetScheme(),
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-discovery"),
	)

	cronManager := scheduler.NewScheduler(ctrl.Log.WithName("scheduler"), health)

	// Initialize authentication provider (OIDC or GitHub OAuth)
	var authProvider ui.AuthProvider
	oidcIssuer := config.GetValue("OIDC_ISSUER_URL")
	oidcClientID := config.GetValue("OIDC_CLIENT_ID")
	oidcClientSecret := config.GetValue("OIDC_CLIENT_SECRET")
	githubClientID := config.GetValue("GITHUB_CLIENT_ID")
	githubClientSecret := config.GetValue("GITHUB_CLIENT_SECRET")

	if oidcIssuer != "" && oidcClientID != "" && oidcClientSecret != "" {
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer oidcCancel()
		oidcAuth, oidcErr := ui.NewOIDCAuth(oidcCtx, ui.OIDCConfig{
			IssuerURL:          oidcIssuer,
			ClientID:           oidcClientID,
			ClientSecret:       oidcClientSecret,
			RedirectURL:        config.GetValue("OIDC_REDIRECT_URL"),
			SessionSecret:      config.GetValue("OIDC_SESSION_SECRET"),
			InsecureSkipVerify: config.GetValue("OIDC_INSECURE_SKIP_VERIFY") == "true",
			LogoutURL:          config.GetValue("OIDC_LOGOUT_URL"),
		}, ctrl.Log.WithName("oidc"))
		assert.NoError(oidcErr, "failed to initialize OIDC provider")
		authProvider = oidcAuth
		ctrl.Log.WithName("auth").Info("OIDC authentication enabled", "issuer", oidcIssuer)
	} else if githubClientID != "" && githubClientSecret != "" {
		ghAuth, ghErr := ui.NewGitHubOAuth(ui.GitHubOAuthConfig{
			ClientID:      githubClientID,
			ClientSecret:  githubClientSecret,
			RedirectURL:   config.GetValue("GITHUB_REDIRECT_URL"),
			SessionSecret: config.GetValue("GITHUB_SESSION_SECRET"),
		}, ctrl.Log.WithName("github-oauth"))
		assert.NoError(ghErr, "failed to initialize GitHub OAuth provider")
		authProvider = ghAuth
		ctrl.Log.WithName("auth").Info("GitHub OAuth authentication enabled")
	} else {
		ctrl.Log.WithName("auth").Info("No authentication configured, UI access is unauthenticated")
	}

	// UI and webhook servers run on all replicas
	uiServer := ui.NewServer(jobMgr, discovery, cronManager, ctrl.Log.WithName("ui-server"), health, Version, authProvider)
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
	}).SetupWithManager(mgr)
	assert.NoError(err, "failed to setup manager")

	err = mgr.Start(ctx)
	assert.NoError(err, "failed to start manager")
}
