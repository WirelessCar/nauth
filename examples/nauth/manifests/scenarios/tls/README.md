# TLS scenario

This scenario connects NAuth to a NATS server that uses a private CA and a TLS-first handshake
(`tls { handshake_first: true }`). See the [TLS guide](https://nauth.io/guides/tls/) for all options.

## Prerequisites

Create these resources in the `tls-example` namespace before applying the manifest:

1. **Operator signing key** (`my-operator-signing-key`) with key `seed`.
2. **System account user credentials** (`my-system-account-creds`) with key `user.creds`.
3. **CA bundle** (`nats-ca`) with the PEM CA that issued the server certificate:

   ```bash
   kubectl create configmap nats-ca -n tls-example --from-file=ca.crt=./ca.crt
   ```

   A Secret works too: set `caRef.kind: Secret` instead of `ConfigMap`.

4. **Client certificate** (optional, for mutual TLS): a `kubernetes.io/tls` Secret, then uncomment
   `clientCertSecretRef` in the manifest.

   ```bash
   kubectl create secret tls nauth-client-tls -n tls-example --cert=client.crt --key=client.key
   ```

## Apply

```bash
kubectl apply -f tls.yaml
kubectl get natscluster -n tls-example tls-cluster
```

`Ready=True` means NAuth reached the system account over TLS. Otherwise the `Ready` condition message
names the cause, for example an unknown CA or a `handshakeFirst` mismatch.
