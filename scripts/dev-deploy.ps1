param(
	[ValidateSet("all", "edge", "authority")]
	[string]$Component = "all",
	[ValidateSet("auto", "docker", "acr")]
	[string]$Builder = "auto",
	[string]$Namespace = "ambience-dev",
	[string]$EdgeDeployment = "ambience-edge",
	[string]$AuthorityStatefulSet = "ambience-authority",
	[string]$ArgoNamespace = "argocd",
	[string]$ArgoAppName = "ambience-dev",
	[string]$RegistryName = "romainecr",
	[string]$ImageRepository = "ambience"
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

function Test-DockerAvailable {
	try {
		docker version --format '{{.Server.Version}}' *> $null
		return $true
	} catch {
		return $false
	}
}

function Test-AzAvailable {
	try {
		az account show --query id -o tsv *> $null
		return $true
	} catch {
		return $false
	}
}

function Resolve-BuildMode {
	param([string]$Requested)

	switch ($Requested) {
		"docker" {
			if (-not (Test-DockerAvailable)) {
				throw "Builder 'docker' requested but Docker is not available."
			}
			return "docker"
		}
		"acr" {
			if (-not (Test-AzAvailable)) {
				throw "Builder 'acr' requested but Azure CLI is not available or not logged in."
			}
			return "acr"
		}
		default {
			if (Test-DockerAvailable) {
				return "docker"
			}
			if (Test-AzAvailable) {
				return "acr"
			}
			throw "No build path available: Docker is unavailable and Azure CLI is not ready for ACR builds."
		}
	}
}

function Test-RegistryTagExists {
	param(
		[string]$RegistryName,
		[string]$Repository,
		[string]$Tag
	)

	try {
		$match = az acr repository show-tags `
			--name $RegistryName `
			--repository $Repository `
			--query "[?@=='$Tag'] | [0]" `
			-o tsv 2>$null
		return $match -eq $Tag
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
$registryLoginServer = "$RegistryName.azurecr.io"

$head = (git -C $repoRoot rev-parse --short HEAD).Trim()
$stamp = Get-Date -Format "yyyyMMddHHmmss"
$tag = "devloop-$head-$stamp"
$image = "$registryLoginServer/${ImageRepository}:$tag"
$buildMode = Resolve-BuildMode -Requested $Builder

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

switch ($buildMode) {
	"docker" {
		docker build -t $image $repoRoot
		$buildSeconds = Format-Seconds $step

		$step.Restart()
		az acr login --name $RegistryName | Out-Null
		docker push $image
		$pushSeconds = Format-Seconds $step
	}
	"acr" {
		az acr build -r $RegistryName -t "${ImageRepository}:$tag" $repoRoot
		$buildSeconds = Format-Seconds $step
		$pushSeconds = 0.0
	}
}

if (-not (Test-RegistryTagExists -RegistryName $RegistryName -Repository $ImageRepository -Tag $tag)) {
	throw "Image tag '$tag' was not found in ACR after the build. Refusing to patch workloads."
}

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
Write-Output "BUILD_MODE=$buildMode"
Write-Output "TAG=$tag"
Write-Output "IMAGE=$image"
Write-Output "ARGO_APP_PRESENT=$argoAppPresent"
Write-Output "ARGO_DRIFT_EXPECTED=$argoAppPresent"
Write-Output "REGISTRY_TAG_VERIFIED=true"
Write-Output "EDGE_TAG=$nextEdgeTag"
Write-Output "AUTHORITY_TAG=$nextAuthorityTag"
Write-Output "BUILD_SECONDS=$buildSeconds"
Write-Output "PUSH_SECONDS=$pushSeconds"
Write-Output "PATCH_SECONDS=$patchSeconds"
Write-Output "EDGE_SECONDS=$edgeSeconds"
Write-Output "AUTHORITY_SECONDS=$authoritySeconds"
Write-Output "TOTAL_SECONDS=$([math]::Round($total.Elapsed.TotalSeconds, 2))"
