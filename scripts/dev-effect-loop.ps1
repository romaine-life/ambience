param(
	[ValidateSet("auto", "docker", "acr")]
	[string]$Builder = "auto",
	[string]$Namespace = "ambience-dev",
	[string]$EdgeSelector = "app=ambience,component=edge",
	[string]$EdgeContainer = "web-sync",
	[string]$EdgeRemoteDir = "/var/run/ambience-web-override"
)

$ErrorActionPreference = "Stop"

$deployScript = Join-Path $PSScriptRoot "dev-deploy.ps1"
$loopScript = Join-Path $PSScriptRoot "dev-loop.ps1"

$total = [System.Diagnostics.Stopwatch]::StartNew()
$step = [System.Diagnostics.Stopwatch]::StartNew()

Write-Host "effect loop: rolling authority image first"
& $deployScript -Component authority -Builder $Builder -Namespace $Namespace
$authoritySeconds = [math]::Round($step.Elapsed.TotalSeconds, 2)

$step.Restart()
Write-Host "effect loop: syncing web overrides into the live edge pod"
& $loopScript -Namespace $Namespace -Selector $EdgeSelector -Container $EdgeContainer -RemoteDir $EdgeRemoteDir -Once
$webSyncSeconds = [math]::Round($step.Elapsed.TotalSeconds, 2)

$total.Stop()

Write-Output "MODE=effect"
Write-Output "AUTHORITY_SECONDS=$authoritySeconds"
Write-Output "WEB_SYNC_SECONDS=$webSyncSeconds"
Write-Output "TOTAL_SECONDS=$([math]::Round($total.Elapsed.TotalSeconds, 2))"
