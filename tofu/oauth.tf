# Ambience user-auth Entra app.
#
# This is app-owned infrastructure: the registration exists for Ambience's
# browser control sign-in, while the public client id is published to Key Vault
# for the chart to project into the authority pod.

resource "azuread_application" "oauth_registration" {
  display_name     = "ambience-oauth"
  sign_in_audience = "AzureADMyOrg"

  owners = [data.azuread_client_config.current.object_id]

  api {
    requested_access_token_version = 2
  }

  # The web platform permits the wildcard callback used by per-issue preview
  # hosts. Ambience consumes an id_token from the browser and validates it
  # server-side before minting the live-control session cookie.
  web {
    redirect_uris = [
      for hostname in var.hostnames : "https://${hostname}/auth/callback"
    ]

    implicit_grant {
      access_token_issuance_enabled = false
      id_token_issuance_enabled     = true
    }
  }

  required_resource_access {
    resource_app_id = "00000003-0000-0000-c000-000000000000" # Microsoft Graph

    resource_access {
      id   = "e1fe6dd8-ba31-4d61-89e7-88639da4683d" # User.Read
      type = "Scope"
    }
  }
}

resource "azuread_service_principal" "oauth_service_principal" {
  client_id = azuread_application.oauth_registration.client_id
}

resource "azurerm_key_vault_secret" "oauth_client_id" {
  name         = "ambience-oauth-client-id"
  value        = azuread_application.oauth_registration.client_id
  key_vault_id = data.azurerm_key_vault.main.id
}
