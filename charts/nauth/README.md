# NAuth

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for scheduling the NAuth operator Pod. |
| crds.install | bool | `true` | Install and upgrade NAuth CustomResourceDefinitions as part of this chart release. |
| crds.keep | bool | `true` | Keep NAuth CustomResourceDefinitions when this chart release is uninstalled. |
| extraResources | list | `[]` | Additional Kubernetes resources to render with the chart. Values are templated before rendering. |
| fullnameOverride | string | `""` | Override the full generated resource name. Defaults to `<Release.Name>-<Chart.Name>`. |
| global.labels | object | `{}` | Additional labels to add to templated NAuth chart resources. |
| image.pullPolicy | string | `"IfNotPresent"` | Kubernetes image pull policy for the NAuth operator container. |
| image.registry | string | `"ghcr.io/wirelesscar"` | Container image registry for the NAuth operator. |
| image.repository | string | `"nauth-operator"` | Container image repository for the NAuth operator. |
| image.tag | string | appVersion | Override the container image tag. Defaults to the chart app version when empty. |
| livenessProbe | object | `{"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":15,"periodSeconds":20}` | Liveness probe for the NAuth operator container. |
| logging.format | string | `""` | Operator log output format. Leave empty to use the operator default. Set to `json` for structured log ingestion. Supported values: `text`, `json`. |
| logging.level | string | `""` | Operator log level. Supported values: `debug`, `info`, `warn`, `error`. |
| monitoring.enabled | bool | `false` | Expose controller-runtime Prometheus metrics on `/metrics` for direct scraping or collection through a Prometheus receiver. |
| monitoring.serviceMonitor.enabled | bool | `false` | Create Prometheus Operator ServiceMonitor and PrometheusRule resources. Requires the ServiceMonitor and PrometheusRule CRDs. |
| nameOverride | string | `""` | Override the chart name used in generated resource names. |
| namespace.nameOverride | string | `""` | Override the namespace rendered into namespaced resources. Defaults to the Helm release namespace. |
| namespaced | bool | `false` | Limit the operator to the configured namespace instead of watching all namespaces. |
| nats.allowAccountNatsClusterRebind | bool | `false` | Allow existing Accounts to change their NatsCluster binding. Disabled by default. |
| nats.clusterRef | object | `{"name":"","namespace":"","optional":false}` | Operator-level NatsCluster reference. Set `name` to bind the operator to one NATS cluster. |
| nats.clusterRef.name | string | `""` | NatsCluster resource name. Leave empty to disable operator-level binding. |
| nats.clusterRef.namespace | string | `""` | NatsCluster resource namespace. When empty and `name` is set, the chart namespace is used. |
| nats.clusterRef.optional | bool | `false` | Whether account-level `spec.natsClusterRef` values may override this operator-level cluster. |
| nodeSelector | object | `{}` | Node selector for scheduling the NAuth operator Pod. |
| podAnnotations | object | `{}` | Annotations to add to the NAuth operator Pod. |
| podLabels | object | `{}` | Additional labels to add to the NAuth operator Pod. |
| podSecurityContext | object | `{"runAsNonRoot":true}` | Pod security context for the NAuth operator Pod. |
| readinessProbe | object | `{"httpGet":{"path":"/readyz","port":8081},"initialDelaySeconds":5,"periodSeconds":10}` | Readiness probe for the NAuth operator container. |
| replicaCount | int | `1` | Number of NAuth operator replicas. |
| resources | object | `{}` | Resource requests and limits for the NAuth operator container. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsGroup":65532,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Container security context for the NAuth operator container. |
| serviceAccount.annotations | object | `{}` | Annotations to add to the ServiceAccount. |
| serviceAccount.create | bool | `true` | Create a ServiceAccount for the NAuth operator. |
| serviceAccount.nameOverride | string | `""` | Override the ServiceAccount name. Defaults to the generated full name when empty. |
| terminationGracePeriodSeconds | int | `10` | Termination grace period for the NAuth operator Pod, in seconds. |
| tolerations | list | `[]` | Tolerations for scheduling the NAuth operator Pod. |
| volumeMounts | list | `[]` | Additional volume mounts for the NAuth operator container. |
| volumes | list | `[]` | Additional volumes for the NAuth operator Pod. |
