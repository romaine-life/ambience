# Cosmos DB NoSQL database (per-app; the account itself is provisioned by
# infra-bootstrap as a shared resource). Mirrors glimmung/tofu/db.tf and the
# convention noted in infra-bootstrap/tofu/cosmos-serverless.tf — narrow
# per-app database scope, not shared account scope.
#
# Single container holding the shared atmosphere snapshot. Partition key is
# `/id`; the runtime writes one document with id="shared". A single doc per
# partition keeps point-reads at 1 RU and point-writes at the document size's
# RU cost (~10 for the current persistedAtmosphere shape).

resource "azurerm_cosmosdb_sql_database" "ambience" {
  name                = "ambience"
  resource_group_name = local.infra.resource_group_name
  account_name        = data.azurerm_cosmosdb_account.infra.name
  lifecycle {
    ignore_changes = [throughput]
  }
}

resource "azurerm_cosmosdb_sql_container" "atmosphere" {
  name                = "atmosphere"
  resource_group_name = local.infra.resource_group_name
  account_name        = data.azurerm_cosmosdb_account.infra.name
  database_name       = azurerm_cosmosdb_sql_database.ambience.name
  partition_key_paths = ["/id"]

  indexing_policy {
    indexing_mode = "consistent"
    included_path {
      path = "/*"
    }
  }
}
