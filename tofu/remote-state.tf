# References to shared infrastructure provisioned by infra-bootstrap. This app
# owns its Entra registration, but writes public runtime config into the shared
# Key Vault so External Secrets can project it into Kubernetes.

locals {
  infra = {
    resource_group_name = "infra"
  }
}

data "azuread_client_config" "current" {}

data "azurerm_resource_group" "infra" {
  name = local.infra.resource_group_name
}

data "azurerm_key_vault" "main" {
  name                = var.key_vault_name
  resource_group_name = local.infra.resource_group_name
}

data "azurerm_cosmosdb_account" "infra" {
  name                = "infra-cosmos-serverless"
  resource_group_name = local.infra.resource_group_name
}

data "terraform_remote_state" "infra_bootstrap" {
  backend = "azurerm"

  config = {
    resource_group_name  = "infra"
    storage_account_name = "nelsontofu"
    container_name       = "tfstate"
    key                  = "infra-bootstrap.tfstate"
    use_oidc             = true
  }
}

locals {
  aks_oidc_issuer_url = data.terraform_remote_state.infra_bootstrap.outputs.aks_oidc_issuer_url
}
