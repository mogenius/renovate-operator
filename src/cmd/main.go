package main

import (
	"fmt"
	"strconv"

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

func adaptKubeConfig(cfg *rest.Config) {
	cfg.QPS = 50
	cfg.Burst = 100
}

func main() {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{
			Key:      "CUSTOM_CSS_FILE_PATH",
			Optional: true,
		},
		{
			Key:      "CUSTOM_FAVICON_FILE_PATH",
			Optional: true,
		},
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
	})
	assert.NoError(err, "failed to initialize config module")

	cfg := ctrl.GetConfigOrDie()
	adaptKubeConfig(cfg)

	ctrl.SetLogger(zap.New())

	metricStore.Register(ctrlmetrics.Registry)

	watchNamespace := config.GetValue("WATCH_NAMESPACE")
	mgrOptions := ctrl.Options{
		Scheme:         nil,
		LeaderElection: false,
		Cache:          cache.Options{DefaultNamespaces: map[string]cache.Config{watchNamespace: {}}},
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

	executor := renovate.NewRenovateExecutor(
		mgr.GetScheme(),
		jobMgr,
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-executor"),
		health,
	)
	err = executor.Start(ctx)
	assert.NoError(err, "failed to start executor")

	discovery := renovate.NewDiscoveryAgent(
		mgr.GetScheme(),
		mgr.GetClient(),
		ctrl.Log.WithName("renovate-discovery"),
	)

	cronManager := scheduler.NewScheduler(ctrl.Log.WithName("scheduler"), health)
	cronManager.Start()

	uiServer := ui.NewServer(jobMgr, discovery, cronManager, ctrl.Log.WithName("ui-server"), health)
	uiServer.Run()

	if config.GetValue("WEBHOOK_SERVER_ENABLED") != "false" {
		webhookServer := webhook.NewWebookServer(jobMgr, ctrl.Log.WithName("webhook"))
		webhookServer.Run()
	}

	err = (&controllers.RenovateJobReconciler{
		Scheduler: cronManager,
		Manager:   jobMgr,
		Discovery: discovery,
	}).SetupWithManager(mgr)
	assert.NoError(err, "failed to setup manager")

	err = mgr.Start(ctx)
	assert.NoError(err, "failed to start manager")
}
