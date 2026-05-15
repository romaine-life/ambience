# Workload identity for ambience.
#
# Mirrors the per-app convention established by glimmung/tofu/identity.tf and
# documented in infra-bootstrap/tofu/cosmos-serverless.tf: each app owns a
# user-assigned identity with Cosmos data-plane scope narrowed to its own
# database, not the account. A pod compromise in this app cannot reach any
# other app's data on the shared infra-cosmos-serverless account.

resource "azurerm_resource_group" "ambience" {
  name     = "ambience"
  location = data.azurerm_resource_group.infra.location
}

resource "azurerm_user_assigned_identity" "ambience" {
  name                = "ambience-identity"
  resource_group_name = azurerm_resource_group.ambience.name
  location            = azurerm_resource_group.ambience.location
}

# Cosmos DB Built-in Data Contributor (00000000-0000-0000-0000-000000000002)
# scoped to the ambience database, not the account. `scope` uses the Cosmos
# data-plane path scheme (`/dbs/<name>`) — the ARM resource ID's
# `/sqlDatabases/<name>` form is rejected by the Cosmos service.
resource "azurerm_cosmosdb_sql_role_assignment" "ambience_cosmos" {
  resource_group_name = local.infra.resource_group_name
  account_name        = data.azurerm_cosmosdb_account.infra.name
  role_definition_id  = "${data.azurerm_cosmosdb_account.infra.id}/sqlRoleDefinitions/00000000-0000-0000-0000-000000000002"
  principal_id        = azurerm_user_assigned_identity.ambience.principal_id
  scope               = "${data.azurerm_cosmosdb_account.infra.id}/dbs/${azurerm_cosmosdb_sql_database.ambience.name}"
}

# Federated to the default ServiceAccount in the prod ambience namespace.
# Per-slot fed creds (system:serviceaccount:ambience-slot-N:default) are
# NOT declared here — Glimmung's NativeWorkloadIdentityService reconciles
# them dynamically from the ambience project's `native_standby_workload_identity`
# metadata, matched to slot count and slot prefix. See
# glimmung/internal/server/native_workload_identities.go.
resource "azurerm_federated_identity_credential" "ambience" {
  name                = "aks-ambience"
  resource_group_name = azurerm_resource_group.ambience.name
  parent_id           = azurerm_user_assigned_identity.ambience.id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = local.aks_oidc_issuer_url
  subject             = "system:serviceaccount:ambience:default"
}

# The slot fed creds (aks-ambience-slot-1..5) were briefly managed here
# before ambience's project metadata declared native_standby_workload_identity
# in Glimmung. Glimmung's reconciler now owns them. Hand state ownership
# over without destroying the Azure resources — Glimmung's next reconcile
# adopts them by template match (same identity, name, subject, audiences).
removed {
  from = azurerm_federated_identity_credential.ambience_slot
  lifecycle {
    destroy = false
  }
}

output "ambience_identity_client_id" {
  value       = azurerm_user_assigned_identity.ambience.client_id
  description = "client_id of ambience-identity. Pin into chart/ambience/values.yaml as authority.cosmos.serviceAccountClientId."
}
