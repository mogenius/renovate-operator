package github

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TokenRenewalWorker interface {
	CreateOrUpdateWorker(job *api.RenovateJob)
	DeleteWorker(name, namespace string)
}

type tokenRenewalWorker struct {
	activeWorker map[string]*api.RenovateJob
	logger       logr.Logger
	appToken     GithubAppToken
	client       client.Client
}

func StartTokenRenewalWorker(ctx context.Context, logger logr.Logger, appToken GithubAppToken, client client.Client) *tokenRenewalWorker {
	worker := &tokenRenewalWorker{
		activeWorker: make(map[string]*api.RenovateJob),
		logger:       logger,
		appToken:     appToken,
		client:       client,
	}
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				worker.doWork()
			}
		}
	}()
	return worker
}

func (worker *tokenRenewalWorker) CreateOrUpdateWorker(job *api.RenovateJob) {
	if job.Spec.GithubAppReference == nil {
		// not a github app job -> ignore
		return
	}

	worker.activeWorker[job.Fullname()] = job
}

func (worker *tokenRenewalWorker) DeleteWorker(name, namespace string) {

	fullname := namespace + "-" + name
	_, exists := worker.activeWorker[fullname]
	if exists {
		delete(worker.activeWorker, fullname)

		secret := &corev1.Secret{}
		secret.Namespace = namespace
		secret.Name = GetNameForGithubAppSecretFromJobName(name)
		err := worker.client.Delete(context.Background(), secret)
		if err != nil {
			worker.logger.Error(err, "failed to delete github app secret for job", "job", fullname)
		}
	}
}

func (worker *tokenRenewalWorker) doWork() {
	ctx := context.Background()
	for key, value := range worker.activeWorker {
		worker.logger.Info("reneweing github app token for job", "job", key)
		worker.handleJob(ctx, value)
	}
}
func (worker *tokenRenewalWorker) handleJob(ctx context.Context, job *api.RenovateJob) error {
	token, err := worker.appToken.CreateGithubAppTokenFromJob(job)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			"RENOVATE_TOKEN":   []byte(token),
			"GITHUB_COM_TOKEN": []byte(token),
			"GITHUB_COM_USER":  []byte("app"),
		},
	}
	secret.Namespace = job.Namespace
	secret.Name = GetNameForGithubAppSecret(job)

	return worker.client.Create(ctx, secret)
}
