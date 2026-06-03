# Ambience Infrastructure

This directory owns Ambience-specific Azure resources. Shared cluster
infrastructure still lives in `romaine-life/infra-bootstrap`.

## App-Owned Key Vault

Ambience provisions `ng6-ambience` in this stack and writes
`ambience-oauth-client-id` there. The chart defines the matching
`ClusterSecretStore` so External Secrets can project the client ID into
Kubernetes from the app-owned vault. Ambience CI's Key Vault data-plane access
comes from the subscription-scope
`Key Vault Administrator` grant assigned by `infra-bootstrap`; this stack only
grants runtime read access to External Secrets.

## Existing OAuth Registration Migration

`ambience-oauth` was originally declared in `infra-bootstrap`. Apply the
matching `infra-bootstrap` change first so that state forgets the old resources
without deleting them. Then import the live objects here before the first
Ambience tofu apply:

```sh
tofu import azuread_application.oauth <ambience-oauth application object id>
tofu import azuread_service_principal.oauth <ambience-oauth service principal object id>
```

After import, `tofu plan` should show no replacement for the Entra application.
