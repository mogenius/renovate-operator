# PreUpgrade and PostUpgrade Hooks

The Renovate Operator supports optional `preUpgrade` and `postUpgrade` hooks that allow you to run custom logic before and after Renovate executions.

## Overview

Hooks are implemented as separate Kubernetes Jobs that run:
- **preUpgrade**: Before the Renovate job starts
- **postUpgrade**: After the Renovate job completes successfully

Each hook runs as an independent container with its own configuration, allowing you to use any Docker image and run any commands.

## Use Cases

### PreUpgrade Hooks
- **Environment validation**: Check that required credentials, tokens, or services are available
- **Backup operations**: Create backups of configurations before updates
- **Pre-checks**: Validate repository state, branch protection rules, or CI status
- **Setup tasks**: Download additional config files, prepare working directories
- **Notifications**: Send "starting update" notifications to Slack, email, etc.

### PostUpgrade Hooks
- **Notifications**: Send completion status to webhooks, Slack, email, or monitoring systems
- **Log aggregation**: Upload Renovate logs to external storage or logging services
- **Cleanup operations**: Remove temporary files, clear caches
- **Verification**: Run additional checks on the results of the Renovate run
- **Trigger downstream workflows**: Start CI/CD pipelines, run tests, or update dashboards

## Configuration

Hooks are configured in the `RenovateJob` spec using the `preUpgrade` and `postUpgrade` fields.

### Basic Example

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: example-with-hooks
spec:
  schedule: "0 2 * * *"
  image: ghcr.io/renovatebot/renovate:latest
  parallelism: 2
  secretRef: renovate-secret

  # Run validation before renovate
  preUpgrade:
    image: alpine:3
    command: ["/bin/sh", "-c"]
    args:
      - |
        echo "Running pre-upgrade validation..."
        if [ -z "$RENOVATE_TOKEN" ]; then
          echo "Error: RENOVATE_TOKEN not set"
          exit 1
        fi
        echo "Validation passed"
    envFrom:
      - secretRef:
          name: renovate-secret

  # Send notification after renovate
  postUpgrade:
    image: curlimages/curl:latest
    command: ["/bin/sh", "-c"]
    args:
      - |
        echo "Sending completion notification..."
        curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
          -H 'Content-Type: application/json' \
          -d '{"text": "Renovate completed for project"}'
    failOnError: false  # Don't fail the job if notification fails
```

## Hook Configuration Options

### Required Fields

- **`image`** (string): The Docker image to use for the hook container

### Optional Fields

- **`command`** ([]string): The command to execute (e.g., `["/bin/sh", "-c"]`)
- **`args`** ([]string): Arguments for the command
- **`env`** ([]EnvVar): Environment variables for the hook
- **`envFrom`** ([]EnvFromSource): Environment variables from secrets or configmaps
- **`failOnError`** (*bool): Whether hook failure should block execution (default: `true`)
- **`resources`** (ResourceRequirements): CPU and memory requests/limits for the hook
- **`volumeMounts`** ([]VolumeMount): Volume mounts for the hook container

## Failure Handling

The `failOnError` flag controls what happens when a hook fails:

### PreUpgrade Hooks

**`failOnError: true`** (default)
```yaml
preUpgrade:
  image: alpine:3
  command: ["/bin/sh", "-c"]
  args: ["exit 1"]  # This will fail
  failOnError: true
```
- Hook failure → Project status = `failed`
- Renovate job **does not run**
- Use this when the hook is critical (e.g., credential validation)

**`failOnError: false`**
```yaml
preUpgrade:
  image: alpine:3
  command: ["/bin/sh", "-c"]
  args: ["exit 1"]  # This will fail
  failOnError: false
```
- Hook failure → Warning logged
- Renovate job **continues normally**
- Use this for non-critical tasks (e.g., optional notifications)

### PostUpgrade Hooks

**`failOnError: true`** (default)
```yaml
postUpgrade:
  image: curlimages/curl:latest
  command: ["/bin/sh", "-c"]
  args: ["curl https://invalid-url"]  # This will fail
  failOnError: true
```
- Hook failure → Project status = `failed` (even if Renovate succeeded)
- Use this when post-processing is critical

**`failOnError: false`**
```yaml
postUpgrade:
  image: curlimages/curl:latest
  command: ["/bin/sh", "-c"]
  args: ["curl https://invalid-url"]  # This will fail
  failOnError: false
```
- Hook failure → Warning logged
- Project status = `completed` (uses Renovate job's status)
- Use this for best-effort operations (e.g., notifications, metrics)

**Note**: If the Renovate job fails, the postUpgrade hook is **skipped entirely** and the project is marked as `failed`.

## Data Exchange Between Hooks and Renovate

Hooks share the `/tmp` volume with the Renovate container, enabling data exchange:

```yaml
preUpgrade:
  image: alpine:3
  command: ["/bin/sh", "-c"]
  args:
    - |
      echo "custom-config-value" > /tmp/config.txt
      echo "PreUpgrade completed"

# Renovate can read /tmp/config.txt

postUpgrade:
  image: alpine:3
  command: ["/bin/sh", "-c"]
  args:
    - |
      if [ -f /tmp/renovate-logs.txt ]; then
        cat /tmp/renovate-logs.txt
      fi
      echo "PostUpgrade completed"
```

### Available Volumes

1. **`/tmp` (EmptyDir)**: Always available, shared between all phases
2. **`ExtraVolumes`**: Any volumes defined in `spec.extraVolumes` are also available

You can customize volume mounts for hooks:

```yaml
spec:
  extraVolumes:
    - name: shared-data
      persistentVolumeClaim:
        claimName: my-pvc

  preUpgrade:
    image: alpine:3
    volumeMounts:
      - name: shared-data
        mountPath: /data
      - name: tmp
        mountPath: /tmp
    command: ["/bin/sh", "-c"]
    args: ["cp /data/config.json /tmp/"]
```

## Environment Variables

Hooks can access environment variables from:

1. **Direct `env` field**:
```yaml
preUpgrade:
  image: alpine:3
  env:
    - name: LOG_LEVEL
      value: "debug"
    - name: PROJECT_NAME
      value: "my-project"
```

2. **Secrets via `envFrom`**:
```yaml
preUpgrade:
  image: alpine:3
  envFrom:
    - secretRef:
        name: renovate-secret
    - configMapRef:
        name: renovate-config
```

## Resource Management

Specify CPU and memory for hooks:

```yaml
preUpgrade:
  image: my-heavy-validation:latest
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
```

If not specified, hooks have no resource requests or limits (cluster defaults apply).

## Scheduling and Security

Hooks inherit scheduling and security settings from the `RenovateJob` spec:

- **Node Selector**: `spec.nodeSelector`
- **Affinity**: `spec.affinity`
- **Tolerations**: `spec.tolerations`
- **Security Context**: `spec.securityContext`
- **Service Account**: `spec.serviceAccount`
- **Image Pull Secrets**: `spec.imagePullSecrets`

This ensures hooks run on the same nodes and with the same permissions as Renovate jobs.

## Advanced Examples

### PreUpgrade: Validate GitHub Token

```yaml
preUpgrade:
  image: curlimages/curl:latest
  command: ["/bin/sh", "-c"]
  args:
    - |
      echo "Validating GitHub token..."
      response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: token $GITHUB_TOKEN" \
        https://api.github.com/user)

      if [ "$response" = "200" ]; then
        echo "Token is valid"
        exit 0
      else
        echo "Token validation failed (HTTP $response)"
        exit 1
      fi
  envFrom:
    - secretRef:
        name: renovate-secret
  failOnError: true
```

### PostUpgrade: Send Slack Notification

```yaml
postUpgrade:
  image: curlimages/curl:latest
  command: ["/bin/sh", "-c"]
  args:
    - |
      WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
      PROJECT="${PROJECT_NAME:-unknown}"

      curl -X POST "$WEBHOOK_URL" \
        -H 'Content-Type: application/json' \
        -d "{
          \"text\": \"✅ Renovate completed for project: $PROJECT\",
          \"username\": \"Renovate Bot\"
        }"
  env:
    - name: PROJECT_NAME
      value: "my-repo"
  failOnError: false
```

### PreUpgrade: Download Configuration from S3

```yaml
preUpgrade:
  image: amazon/aws-cli:latest
  command: ["/bin/sh", "-c"]
  args:
    - |
      echo "Downloading config from S3..."
      aws s3 cp s3://my-bucket/renovate-config.json /tmp/config.json
      echo "Config downloaded successfully"
  env:
    - name: AWS_ACCESS_KEY_ID
      valueFrom:
        secretKeyRef:
          name: aws-credentials
          key: access-key-id
    - name: AWS_SECRET_ACCESS_KEY
      valueFrom:
        secretKeyRef:
          name: aws-credentials
          key: secret-access-key
  volumeMounts:
    - name: tmp
      mountPath: /tmp
```

### PostUpgrade: Upload Logs to S3

```yaml
postUpgrade:
  image: amazon/aws-cli:latest
  command: ["/bin/sh", "-c"]
  args:
    - |
      TIMESTAMP=$(date +%Y%m%d_%H%M%S)
      LOG_FILE="/tmp/renovate-logs-${TIMESTAMP}.txt"

      if [ -f /tmp/renovate-output.log ]; then
        aws s3 cp /tmp/renovate-output.log \
          "s3://my-logs-bucket/renovate/${PROJECT}/${TIMESTAMP}.log"
        echo "Logs uploaded successfully"
      else
        echo "No logs found to upload"
      fi
  env:
    - name: AWS_ACCESS_KEY_ID
      valueFrom:
        secretKeyRef:
          name: aws-credentials
          key: access-key-id
    - name: AWS_SECRET_ACCESS_KEY
      valueFrom:
        secretKeyRef:
          name: aws-credentials
          key: secret-access-key
    - name: PROJECT
      value: "my-project"
  failOnError: false
```

## Monitoring and Debugging

### Viewing Hook Status

Check the project status in the RenovateJob:

```bash
kubectl get renovatejob example-with-hooks -o yaml
```

Look for the `subStatus` field in the project status:

```yaml
status:
  projects:
    - name: my-project
      status: running
      subStatus: preUpgrade  # or "postUpgrade"
      lastRun: "2024-01-15T10:00:00Z"
```

### Viewing Hook Logs

List hook jobs:

```bash
# List all jobs for a RenovateJob
kubectl get jobs -l renovate-operator.mogenius.com/job-name=<job-name>

# Filter by job type
kubectl get jobs -l renovate-operator.mogenius.com/job-type=preUpgrade
kubectl get jobs -l renovate-operator.mogenius.com/job-type=postUpgrade
```

View hook logs:

```bash
# Get the pod name
kubectl get pods -l job-name=<hook-job-name>

# View logs
kubectl logs <pod-name>
```

### Hook Job Lifecycle

Hooks are implemented as Kubernetes Jobs with the same lifecycle settings as Renovate jobs:

- **ActiveDeadlineSeconds**: Uses `JOB_TIMEOUT_SECONDS` environment variable
- **BackoffLimit**: Uses `JOB_BACKOFF_LIMIT` environment variable
- **TTLSecondsAfterFinished**: Uses `JOB_TTL_SECONDS_AFTER_FINISHED` environment variable
- **DELETE_SUCCESSFUL_JOBS**: If set to `"true"`, completed hook jobs are automatically deleted

## Troubleshooting

### Hook Fails Immediately

**Symptom**: Hook job fails right after creation

**Possible causes**:
1. Image pull error - verify the image exists and is accessible
2. Invalid command or args - check the syntax
3. Missing environment variables - verify secrets/configmaps exist
4. Insufficient resources - check node capacity

**Debug**:
```bash
kubectl describe job <hook-job-name>
kubectl describe pod <hook-pod-name>
```

### Hook Hangs

**Symptom**: Hook runs but never completes

**Possible causes**:
1. Command is waiting for input (hooks don't support interactive commands)
2. Network timeout (no response from external service)
3. Resource limits too low

**Debug**:
```bash
kubectl logs <hook-pod-name> -f
kubectl top pod <hook-pod-name>
```

### Renovate Never Starts

**Symptom**: PreUpgrade completes but Renovate job doesn't start

**Check**:
1. Hook exit code - must be 0 for success
2. `failOnError` setting
3. Parallelism limits - check if max projects are already running

### Data Not Shared Between Hooks

**Symptom**: Files written in preUpgrade are not visible in postUpgrade

**Cause**: Volume mounts not configured correctly

**Solution**: Ensure all hooks mount the same volumes:
```yaml
volumeMounts:
  - name: tmp
    mountPath: /tmp
```

## Security Considerations

1. **Secrets**: Always use Kubernetes secrets for sensitive data, never hardcode
2. **Image Security**: Use trusted images from verified registries
3. **Least Privilege**: Hooks inherit the RenovateJob's service account - ensure it has minimal permissions
4. **Network Policies**: Consider restricting network access for hook containers
5. **Resource Limits**: Set appropriate limits to prevent resource exhaustion

## Best Practices

1. **Keep hooks simple**: Each hook should do one thing well
2. **Use `failOnError` appropriately**: Critical operations should fail, best-effort operations should not
3. **Add logging**: Echo status messages to help with debugging
4. **Test independently**: Test your hook images separately before integrating
5. **Monitor execution time**: Long-running hooks delay the entire workflow
6. **Handle errors gracefully**: Always include error handling in your scripts
7. **Use lightweight images**: Alpine-based images are faster to pull and start

## Limitations

1. **Sequential execution**: Hooks run one at a time (preUpgrade → renovate → postUpgrade)
2. **Per-project isolation**: Each project gets its own hook jobs
3. **No inter-project communication**: Hooks for different projects can't communicate
4. **Timeout applies**: Hooks must complete within `JOB_TIMEOUT_SECONDS`
5. **No retry logic**: Failed hooks are not automatically retried (they follow `JOB_BACKOFF_LIMIT`)

## See Also

- [RenovateJob CRD Reference](../charts/renovate-operator/crd/renovate-operator.mogenius.com_renovatejobs.yaml)
- [Webhooks Documentation](./webhooks/webhook.md)
- [Configuration Options](../README.md)
