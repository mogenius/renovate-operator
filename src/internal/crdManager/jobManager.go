package crdmanager

import (
	"context"
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/internal/utils"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	JOB_LABEL_TYPE        = "renovate-operator.mogenius.com/type"
	JOB_LABEL_RENOVATEJOB = "renovate-operator.mogenius.com/renovatejob"
	JOB_LABEL_PROJECT     = "renovate-operator.mogenius.com/project"
	JOB_LABEL_GENERATION  = "renovate-operator.mogenius.com/generation"
	// JOB_ANNOTATION_PROJECT stores the original project name (may contain slashes etc.)
	// Use this instead of JOB_LABEL_PROJECT when you need the exact CRD status key.
	JOB_ANNOTATION_PROJECT = "renovate-operator.mogenius.com/project"
	// JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY indicates that ProcessDiscoveryJobResult should
	// schedule all non-running projects after reconciling. Set to "true" for cron-triggered
	// discovery; omit or "false" for UI-triggered discovery (project list refresh only).
	JOB_ANNOTATION_SCHEDULE_AFTER_DISCOVERY = "renovate-operator.mogenius.com/schedule-after-discovery"
	// JOB_ANNOTATION_PROCESSED is stamped on a Job after its result has been fully processed.
	// The JobReconciler checks this annotation to skip already-processed jobs on informer resyncs,
	// preventing completed discovery jobs from re-scheduling all projects every ~10h.
	JOB_ANNOTATION_PROCESSED = "renovate-operator.mogenius.com/processed"

	// RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY triggers a discovery run when set to "true".
	// Removed from the RenovateJob once the discovery job is created.
	RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY = "renovate-operator.mogenius.com/discovery"
	// RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL schedules all non-running projects when set to "true".
	// Removed from the RenovateJob after the status update succeeds.
	RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL = "renovate-operator.mogenius.com/schedule-all"
	// RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE schedules specific projects when set to a comma-separated list of project names.
	// Removed from the RenovateJob after the status update succeeds.
	RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE = "renovate-operator.mogenius.com/schedule"
)

type JobType string

const (
	DiscoveryJobType   JobType = "discovery"
	ExecutorJobType    JobType = "executor"
	PreUpgradeJobType  JobType = "preUpgrade"
	PostUpgradeJobType JobType = "postUpgrade"
)

type JobSelector struct {
	RenovateJobName string
	Project         string
	JobType         JobType
	Namespace       string
	// optional generation to filter by - if not provided, the most recent job will be returned
	Generation *string
}

// GetJobByLabel retrieves a single job matching the given labels.
// Returns an error if no job is found.
// If multiple jobs match, the most recently created one is returned.
func GetJobByLabel(ctx context.Context, client crclient.Client, selector JobSelector) (*batchv1.Job, error) {
	allJobs, err := GetJobsByLabel(ctx, client, selector)
	if err != nil {
		return nil, err
	}
	if len(allJobs) == 0 {
		return nil, errors.NewNotFound(batchv1.Resource("jobs"), selector.Project)
	}
	// get the newest job in case there are multiple jobs for the same project (e.g. due to multiple executions)
	var currentJob *batchv1.Job
	var maxGen int64 = -1

	for i := range allJobs {
		genStr, exists := allJobs[i].Labels[JOB_LABEL_GENERATION]
		var gen int64 = 0 // Default to 0 for missing/invalid labels
		if exists {
			parsedGen, err := strconv.ParseInt(genStr, 10, 64)
			if err == nil {
				gen = parsedGen
			}
		}
		// Always select a job, prefer highest generation
		if gen > maxGen || currentJob == nil {
			maxGen = gen
			currentJob = &allJobs[i]
		}
	}
	return currentJob, nil
}

// Retrieve all Jobs by our standard labels
func GetJobsByLabel(ctx context.Context, client crclient.Client, selector JobSelector) ([]batchv1.Job, error) {

	matcher := crclient.MatchingLabels{
		JOB_LABEL_TYPE:        string(selector.JobType),
		JOB_LABEL_RENOVATEJOB: selector.RenovateJobName,
	}
	if selector.JobType == ExecutorJobType && selector.Project != "" {
		matcher[JOB_LABEL_PROJECT] = utils.KubernetesCompatibleProjectName(selector.Project)
	}

	if selector.Generation != nil && *selector.Generation != "" {
		matcher[JOB_LABEL_GENERATION] = *selector.Generation
	}
	jobList := &batchv1.JobList{}
	err := client.List(ctx, jobList, crclient.InNamespace(selector.Namespace), crclient.MatchingLabels(matcher))
	if err != nil {
		return nil, fmt.Errorf("listing jobs with label RenvateJob: %s Project: %s Error: %w", selector.RenovateJobName, selector.Project, err)
	}
	return jobList.Items, nil
}

func DeleteJob(ctx context.Context, client crclient.Client, job *batchv1.Job) error {
	policy := metav1.DeletePropagationBackground
	err := client.Delete(ctx, job, &crclient.DeleteOptions{
		PropagationPolicy: &policy})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete job %s: %w", job.Name, err)
	}
	return nil
}

// MarkJobProcessed stamps JOB_ANNOTATION_PROCESSED on the Job so the JobReconciler
// can skip it on subsequent informer resyncs without re-processing its result.
func MarkJobProcessed(ctx context.Context, c crclient.Client, job *batchv1.Job) error {
	patch := crclient.MergeFrom(job.DeepCopy())
	if job.Annotations == nil {
		job.Annotations = make(map[string]string)
	}
	job.Annotations[JOB_ANNOTATION_PROCESSED] = "true"
	return c.Patch(ctx, job, patch)
}
func CreateJobWithGeneration(ctx context.Context, client crclient.Client, job *batchv1.Job, selector JobSelector) (string, error) {
	assert.Assert(selector.JobType != "", "JobType is required in selector")
	assert.Assert(selector.RenovateJobName != "", "RenovateJobName is required in selector")

	generation := fmt.Sprintf("%d", time.Now().Unix())

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}

	job.Labels[JOB_LABEL_GENERATION] = generation
	job.Labels[JOB_LABEL_TYPE] = string(selector.JobType)
	job.Labels[JOB_LABEL_RENOVATEJOB] = selector.RenovateJobName

	if selector.JobType == ExecutorJobType {
		job.Labels[JOB_LABEL_PROJECT] = utils.KubernetesCompatibleProjectName(selector.Project)
		if job.Annotations == nil {
			job.Annotations = make(map[string]string)
		}
		job.Annotations[JOB_ANNOTATION_PROJECT] = selector.Project
	}

	// Propagate all Job labels to the Pod template so that Pods carry the same
	// operator labels (needed for NetworkPolicies, monitoring selectors, etc.).
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = make(map[string]string)
	}
	for k, v := range job.Labels {
		job.Spec.Template.Labels[k] = v
	}

	// Create immediately - no deletion needed first
	err := client.Create(ctx, job)
	if err != nil {
		return "", fmt.Errorf("creating job with generateName %s: %w", job.GenerateName, err)
	}

	go func() {
		// Create a background context with a timeout
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_ = cleanupOldGenerations(cleanupCtx, client, selector, generation)
	}()

	return generation, nil
}

// Delete jobs that aren't the current generation
func cleanupOldGenerations(ctx context.Context, client crclient.Client, selector JobSelector, currentGen string) error {
	allJobs, err := GetJobsByLabel(ctx, client, selector)
	if err != nil {
		return err
	}

	for _, job := range allJobs {
		gen, exists := job.Labels[JOB_LABEL_GENERATION]

		if !exists || gen != currentGen {
			// This is an old generation - safe to delete
			_ = DeleteJob(ctx, client, &job)
		}
	}

	// TODO: Remove this cleanup logic after we have confidence that the new labels have propagated
	stub := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: selector.RenovateJobName, Namespace: selector.Namespace}}
	name := ""
	if selector.JobType == DiscoveryJobType {
		name = utils.DiscoveryJobName(stub)
	} else {
		name = utils.ExecutorJobName(stub, selector.Project)
	}

	matcher := crclient.MatchingLabels{
		"renovate-operator.mogenius.com/job-type": string(selector.JobType),
		"renovate-operator.mogenius.com/job-name": name,
	}

	jobList := &batchv1.JobList{}
	err = client.List(ctx, jobList, crclient.InNamespace(selector.Namespace), crclient.MatchingLabels(matcher))
	if err != nil {
		return fmt.Errorf("listing jobs for cleanup with label RenvateJob: %s Project: %s Error: %w", selector.RenovateJobName, selector.Project, err)
	}

	for _, job := range jobList.Items {
		_ = DeleteJob(ctx, client, &job)
	}
	return nil
}
