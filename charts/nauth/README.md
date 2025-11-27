# NAuth

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| crds.install | bool | `true` | Indicates if Custom Resource Definitions should be installed and upgraded as part of the release. |
| crds.keep | bool | `true` | Indicates if Custom Resource Definitions should be kept when a release is uninstalled. |
| image.pullPolicy | string | `"IfNotPresent"` | Sets the pull policy for images. |
| image.registry | string | `"ghcr.io/wirelesscar"` | Sets the operator image registry |
| image.repository | string | `"nauth-operator"` | Sets the operator repository |
| image.tag | string | appVersion | Overrides the image tag |
| livenessProbe | object | `{"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":15,"periodSeconds":20}` | This is to setup the liveness and readiness probes more information can be found here: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/ |
| monitoring.enabled | bool | `false` | Enables nauth to use monitoring capabilities. Requires CRD:s to be installed. |
| monitoring.serviceMonitor | object | `{"enabled":false}` | Enables serviceMonitor feature. Requies CRD to be installed beforehand. |
| nameOverride | string | `""` | Override the chart name |
| namespace | object | `{"nameOverride":""}` | Override the namespace |
| namespaced | bool | `false` | If true, limits the scope of nauth to a single namespace. Otherwise, all namespaces will be watched. |
| nats | object | `{"url":"nats://nats.nats.svc.cluster.local:4222"}` | Set the url for your nats server.<BR>The default means nats is deployed in the `nats` namespace. |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` | This is for setting Kubernetes Annotations to a Pod. For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/ |
| podLabels | object | `{}` | This is for setting Kubernetes Labels to a Pod. For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/ |
| podSecurityContext | object | `{"runAsNonRoot":true}` | Pod security context |
| readinessProbe.httpGet.path | string | `"/readyz"` |  |
| readinessProbe.httpGet.port | int | `8081` |  |
| readinessProbe.initialDelaySeconds | int | `5` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| replicaCount | int | `1` | Sets the replicaset count |
| resources | object | `{}` | Setting resources is up to the user. Follows PodSpec. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"runAsGroup":65532,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | SecurityContext of the container |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.automount | bool | `true` | Automatically mount a ServiceAccount's API credentials? |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.nameOverride | string | `""` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template |
| terminationGracePeriodSeconds | int | `10` |  |
| tolerations | list | `[]` |  |
| volumeMounts | list | `[]` |  |
| volumes | list | `[]` |  |
