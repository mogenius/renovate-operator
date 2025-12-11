package github

import (
	"context"
	api "renovate-operator/api/v1alpha1"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	cancel       context.CancelFunc
	ctx          context.Context
}

func NewTokenRenewalWorker(ctx context.Context, logger logr.Logger, appToken GithubAppToken, client client.Client) *tokenRenewalWorker {
	workerCtx, cancel := context.WithCancel(ctx)
	return &tokenRenewalWorker{
		activeWorker: make(map[string]*api.RenovateJob),
		logger:       logger,
		appToken:     appToken,
		client:       client,
		cancel:       cancel,
		ctx:          workerCtx,
	}
}

func (worker *tokenRenewalWorker) Start() {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-worker.ctx.Done():
				return
			case <-ticker.C:
				worker.doWork()
			}
		}
	}()
}

func (worker *tokenRenewalWorker) Stop() {
	if worker.cancel != nil {
		worker.cancel()
	}
}

func (worker *tokenRenewalWorker) CreateOrUpdateWorker(job *api.RenovateJob) {
	if job.Spec.GithubAppReference == nil {
		// not a github app job -> ignore
		return
	}

	secret := &corev1.Secret{}
	err := worker.client.Get(worker.ctx, client.ObjectKey{
		Name:      GetNameForGithubAppSecret(job),
		Namespace: job.Namespace,
	}, secret)

	shouldCreate := false
	if err != nil && errors.IsNotFound(err) {
		// Secret doesn't exist
		shouldCreate = true
		worker.logger.Info("creating new github app token for job", "job", job.Fullname())
	} else if err == nil {
		// Secret exists, check timestamp
		if secret.Annotations != nil {
			if timestampStr, exists := secret.Annotations["renovate-operator.mogenius.com/token-created-at"]; exists {
				if timestamp, parseErr := time.Parse(time.RFC3339, timestampStr); parseErr == nil {
					if time.Since(timestamp) > 30*time.Minute {
						shouldCreate = true
						worker.logger.Info("github app token expired, renewing", "job", job.Fullname(), "age", time.Since(timestamp))
					}
				}
			}
		}
	}

	if shouldCreate {
		err = worker.handleJob(worker.ctx, job)
		if err != nil {
			worker.logger.Error(err, "failed to create github app token for job", "job", job.Fullname())
		}
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
		err := worker.client.Delete(worker.ctx, secret)
		if err != nil {
			worker.logger.Error(err, "failed to delete github app secret for job", "job", fullname)
		}
	}
}

func (worker *tokenRenewalWorker) doWork() {
	for key, value := range worker.activeWorker {
		worker.logger.Info("reneweing github app token for job", "job", key)
		err := worker.handleJob(worker.ctx, value)
		if err != nil {
			worker.logger.Error(err, "failed to renew github app token for job", "job", key)
		}
	}
}
func (worker *tokenRenewalWorker) handleJob(ctx context.Context, job *api.RenovateJob) error {
	token, err := worker.appToken.CreateGithubAppTokenFromJob(job)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.Namespace = job.Namespace
	secret.Name = GetNameForGithubAppSecret(job)

	_, err = controllerutil.CreateOrUpdate(ctx, worker.client, secret, func() error {
		secret.Data = map[string][]byte{
			"RENOVATE_TOKEN": []byte(token),
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations["renovate-operator.mogenius.com/token-created-at"] = time.Now().Format(time.RFC3339)
		return controllerutil.SetControllerReference(job, secret, worker.client.Scheme())
	})

	return err
}
