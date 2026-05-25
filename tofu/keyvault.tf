# Ambience-owned Key Vault for app OAuth material.

resource "azurerm_key_vault" "main" {
  name                       = var.key_vault_name
  resource_group_name        = azurerm_resource_group.ambience.name
  location                   = azurerm_resource_group.ambience.location
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "standard"
  rbac_authorization_enabled = true
  soft_delete_retention_days = 7

  tags = {
    app       = "ambience"
    managedBy = "ambience"
    purpose   = "app-secrets"
  }
}

resource "azurerm_role_assignment" "external_secrets_keyvault" {
  scope                = azurerm_key_vault.main.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = data.azurerm_user_assigned_identity.external_secrets.principal_id
}
