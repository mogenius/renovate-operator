# Pod Scheduling

The Renovate Operator provides flexible scheduling options for the Renovate job pods. You can control where and how your Renovate jobs are scheduled across your Kubernetes cluster using node selectors, affinity rules, tolerations, and topology spread constraints.

## Available Scheduling Options

All scheduling fields are available in the `RenovateJobSpec` and apply to both discovery jobs and renovate execution jobs.

### Node Selector

Use `nodeSelector` to schedule pods on nodes with specific labels. This is the simplest way to constrain pods to nodes.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-node-selector
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  nodeSelector:
    disktype: ssd
    environment: production
```

**Common use cases:**
- Schedule on nodes with fast storage (`disktype: ssd`)
- Separate production and development workloads (`environment: production`)
- Use specific node pools or instance types (`node.kubernetes.io/instance-type: c5.xlarge`)

### Affinity

Affinity provides more expressive scheduling rules than node selectors. You can use node affinity, pod affinity, and pod anti-affinity.

#### Node Affinity

Schedule pods on nodes matching specific criteria:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-node-affinity
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values:
                  - amd64
                  - arm64
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          preference:
            matchExpressions:
              - key: disktype
                operator: In
                values:
                  - ssd
```

#### Pod Affinity

Schedule pods close to other pods (e.g., on the same node or availability zone):

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-pod-affinity
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  affinity:
    podAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
              - key: app
                operator: In
                values:
                  - cache
          topologyKey: kubernetes.io/hostname
```

#### Pod Anti-Affinity

Prevent pods from being scheduled together (spread across nodes):

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-anti-affinity
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 5
  metadata:
    labels:
      app: renovate
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app: renovate
            topologyKey: kubernetes.io/hostname
```

### Tolerations

Tolerations allow pods to be scheduled on nodes with matching taints. This is useful for dedicated node pools or nodes with special hardware.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-tolerations
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "renovate"
      effect: "NoSchedule"
    - key: "high-cpu"
      operator: "Exists"
      effect: "NoExecute"
      tolerationSeconds: 3600
```

**Common taint scenarios:**
- Dedicated node pools (`dedicated=renovate:NoSchedule`)
- Preemptible/spot instances (`preemptible=true:NoSchedule`)
- GPU nodes (`nvidia.com/gpu=present:NoSchedule`)
- Nodes under maintenance (`node.kubernetes.io/unschedulable:NoExecute`)

### Topology Spread Constraints

Control how pods are distributed across topology domains (zones, nodes, regions) to ensure high availability and balanced resource usage.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-topology-spread
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 5
  metadata:
    labels:
      app: renovate
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app: renovate
    - maxSkew: 2
      topologyKey: kubernetes.io/hostname
      whenUnsatisfiable: ScheduleAnyway
      labelSelector:
        matchLabels:
          app: renovate
```

**Use cases:**
- Distribute pods evenly across availability zones
- Balance load across nodes
- Ensure no single zone has too many pods
- Prevent resource hotspots

## Combining Scheduling Options

You can combine multiple scheduling options for fine-grained control:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-advanced-scheduling
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 3
  metadata:
    labels:
      app: renovate
      
  # Simple node selection
  nodeSelector:
    disktype: ssd
  
  # Advanced affinity rules
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values:
                  - amd64
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app: renovate
            topologyKey: kubernetes.io/hostname
  
  # Allow scheduling on dedicated nodes
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "renovate"
      effect: "NoSchedule"
  
  # Spread across zones
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app: renovate
```

## Additional Resources

- [Kubernetes Pod Scheduling](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/)
- [Affinity and Anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity)
- [Taints and Tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)
- [Topology Spread Constraints](https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/)
