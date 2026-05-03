# References to shared infrastructure provisioned by infra-bootstrap. This app
# owns its Entra registration, but writes public runtime config into the shared
# Key Vault so External Secrets can project it into Kubernetes.

locals {
  infra = {
    resource_group_name = "infra"
  }
}

data "azuread_client_config" "current" {}

data "azurerm_key_vault" "main" {
  name                = var.key_vault_name
  resource_group_name = local.infra.resource_group_name
}
