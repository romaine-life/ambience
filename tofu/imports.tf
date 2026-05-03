# The previous migration imported the pre-existing bootstrap-created
# ambience-oauth app/SP. Ambience now creates a fresh app-owned registration
# instead, so forget the adopted Entra objects without requiring Ambience CI to
# mutate or delete bootstrap-created app objects.

removed {
  from = azuread_application.oauth_registration
}

removed {
  from = azuread_service_principal.oauth_service_principal
}
