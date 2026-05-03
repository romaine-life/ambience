# Ambience Infrastructure

This directory owns Ambience-specific Azure resources. Shared cluster
infrastructure still lives in `nelsong6/infra-bootstrap`.

## Existing OAuth Registration Migration

`ambience-oauth` was originally declared in `infra-bootstrap`. Apply the
matching `infra-bootstrap` change first so that state forgets the old resources
without deleting them. Then import the live objects here before the first
Ambience tofu apply:

```sh
tofu import azuread_application.oauth <ambience-oauth application object id>
tofu import azuread_service_principal.oauth <ambience-oauth service principal object id>
tofu import azurerm_key_vault_secret.oauth_client_id https://romaine-kv.vault.azure.net/secrets/ambience-oauth-client-id/<version>
```

After import, `tofu plan` should show no replacement for the Entra application.
