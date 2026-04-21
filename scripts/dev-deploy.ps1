param(
	[ValidateSet("all", "edge", "authority")]
	[string]$Component = "all",
	[string]$Namespace = "ambience-dev",
	[string]$EdgeDeployment = "ambience-edge",
	[string]$AuthorityStatefulSet = "ambience-authority",
	[string]$ArgoNamespace = "argocd",
	[string]$ArgoAppName = "ambience-dev"
)

$ErrorActionPreference = "Stop"

function Get-ImageTagFromResource {
	param(
		[string]$Kind,
		[string]$Name,
		[string]$Namespace
	)

	try {
		$image = kubectl get $Kind $Name -n $Namespace -o jsonpath='{.spec.template.spec.containers[0].image}' 2>$null
		if (-not $image) {
			return ""
		}
		$parts = $image -split ":", 2
		if ($parts.Length -lt 2) {
			return ""
		}
		return $parts[1]
	} catch {
		return ""
	}
}

function Format-Seconds {
	param([System.Diagnostics.Stopwatch]$Stopwatch)
	return [math]::Round($Stopwatch.Elapsed.TotalSeconds, 2)
}

function Test-ArgoApplication {
	param(
		[string]$Namespace,
		[string]$Name
	)

	try {
		kubectl get application $Name -n $Namespace *> $null
		return $true
	} catch {
		return $false
	}
}

function Set-WorkloadImage {
	param(
		[string]$Kind,
		[string]$Name,
		[string]$Namespace,
		[string]$Image
	)

	kubectl set image "$Kind/$Name" "ambience=$Image" -n $Namespace | Out-Null
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

$head = (git -C $repoRoot rev-parse --short HEAD).Trim()
$stamp = Get-Date -Format "yyyyMMddHHmmss"
$tag = "devloop-$head-$stamp"
$image = "romainecr.azurecr.io/ambience:$tag"

$currentEdgeTag = Get-ImageTagFromResource -Kind deployment -Name $EdgeDeployment -Namespace $Namespace
$currentAuthorityTag = Get-ImageTagFromResource -Kind statefulset -Name $AuthorityStatefulSet -Namespace $Namespace

if (-not $currentEdgeTag) { $currentEdgeTag = $tag }
if (-not $currentAuthorityTag) { $currentAuthorityTag = $tag }

$nextEdgeTag = $currentEdgeTag
$nextAuthorityTag = $currentAuthorityTag
switch ($Component) {
	"all" {
		$nextEdgeTag = $tag
		$nextAuthorityTag = $tag
	}
	"edge" {
		$nextEdgeTag = $tag
	}
	"authority" {
		$nextAuthorityTag = $tag
	}
}

$total = [System.Diagnostics.Stopwatch]::StartNew()
$step = [System.Diagnostics.Stopwatch]::StartNew()
$argoAppPresent = Test-ArgoApplication -Namespace $ArgoNamespace -Name $ArgoAppName

docker build -t $image $repoRoot
$buildSeconds = Format-Seconds $step

$step.Restart()
docker push $image
$pushSeconds = Format-Seconds $step

if (-not $argoAppPresent) {
	Write-Warning "Argo application '$ArgoAppName' was not found in namespace '$ArgoNamespace'. Patching live dev workloads anyway."
}

$step.Restart()
if ($Component -in @("all", "edge")) {
	Set-WorkloadImage -Kind "deployment" -Name $EdgeDeployment -Namespace $Namespace -Image $image
}
if ($Component -in @("all", "authority")) {
	Set-WorkloadImage -Kind "statefulset" -Name $AuthorityStatefulSet -Namespace $Namespace -Image $image
}
$patchSeconds = Format-Seconds $step

$edgeSeconds = 0.0
$authoritySeconds = 0.0

if ($Component -in @("all", "edge")) {
	$step.Restart()
	kubectl rollout status "deployment/$EdgeDeployment" -n $Namespace --timeout=120s
	$edgeSeconds = Format-Seconds $step
}

if ($Component -in @("all", "authority")) {
	$step.Restart()
	kubectl rollout status "statefulset/$AuthorityStatefulSet" -n $Namespace --timeout=120s
	$authoritySeconds = Format-Seconds $step
}

$total.Stop()

Write-Output "COMPONENT=$Component"
Write-Output "TAG=$tag"
Write-Output "IMAGE=$image"
Write-Output "ARGO_APP_PRESENT=$argoAppPresent"
Write-Output "ARGO_DRIFT_EXPECTED=$argoAppPresent"
Write-Output "EDGE_TAG=$nextEdgeTag"
Write-Output "AUTHORITY_TAG=$nextAuthorityTag"
Write-Output "BUILD_SECONDS=$buildSeconds"
Write-Output "PUSH_SECONDS=$pushSeconds"
Write-Output "PATCH_SECONDS=$patchSeconds"
Write-Output "EDGE_SECONDS=$edgeSeconds"
Write-Output "AUTHORITY_SECONDS=$authoritySeconds"
Write-Output "TOTAL_SECONDS=$([math]::Round($total.Elapsed.TotalSeconds, 2))"
