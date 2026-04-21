# Valkey

The operator ships with an optional Valkey datastore. This uses the [official Valkey Helm chart](https://github.com/valkey-io/valkey-helm). This deploys a single standalone Valkey instance.

## High availability

The upstream Helm chart currently [does not support](https://github.com/valkey-io/valkey-helm/issues/18) Valkey clustering. Once this is delivered, we will have to update the protocol from `redis://` to `redis+cluster://`.
