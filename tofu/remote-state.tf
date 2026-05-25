# References to shared infrastructure provisioned by infra-bootstrap. This app
# owns its Entra registration and Key Vault; shared infrastructure is limited to
# the resource group, Cosmos account, AKS issuer, and External Secrets identity.

locals {
  infra = {
    resource_group_name = "infra"
  }
}

data "azuread_client_config" "current" {}

data "azurerm_client_config" "current" {}

data "azurerm_resource_group" "infra" {
  name = local.infra.resource_group_name
}

data "azurerm_user_assigned_identity" "external_secrets" {
  name                = "infra-shared-identity"
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
