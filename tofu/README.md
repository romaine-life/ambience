# Ambience Infrastructure

This directory owns Ambience-specific Azure resources. Shared cluster
infrastructure still lives in `romaine-life/infra-bootstrap`.

## App-Owned Key Vault

Ambience provisions `ng6-ambience` in this stack for app-owned secrets. Ambience
CI's Key Vault data-plane access comes from the subscription-scope
`Key Vault Administrator` grant assigned by `infra-bootstrap`.

## Control-admin sign-in

Live-control admins sign in through **auth.romaine.life** (OIDC,
authorization-code + PKCE), like every other romaine.life app — see
`chart/ambience` `authority.controlAuth.oidc` and the `ambience` trusted client
in `romaine-life/auth`. The old standalone single-tenant `ambience-oauth` Entra
app (and its `ambience-oauth-client-id` Key Vault secret) was retired in 2026-06
because it rejected personal Microsoft accounts.
