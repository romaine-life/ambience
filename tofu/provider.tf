# Remote state in Azure Storage (backend config passed by CI). Authentication
# uses GitHub OIDC through this repo's app service principal.

terraform {
  backend "azurerm" {}
}

provider "azurerm" {
  features {}
  use_oidc = true
}
