# Adopt the existing user-facing OAuth resources that were originally
# provisioned by infra-bootstrap, preserving the published client id.

import {
  to = azuread_application.oauth_registration
  id = "/applications/0f8417f4-9ba1-4116-94bd-ae1a18b8f356"
}

import {
  to = azuread_service_principal.oauth_service_principal
  id = "/servicePrincipals/8a400255-bb60-4320-bfa1-b949c3257b08"
}

import {
  to = azurerm_key_vault_secret.oauth_client_id
  id = "https://romaine-kv.vault.azure.net/secrets/ambience-oauth-client-id/dd2ea8c6150748eaadda53389fac360b"
}

# The first Ambience-owned apply ran before these imports existed and created
# duplicate app/SP objects at the old azuread_application.oauth and
# azuread_service_principal.oauth addresses. Those addresses are intentionally
# absent from configuration now, so the next apply destroys only that partial
# duplicate while importing the original registration above.
