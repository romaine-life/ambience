param(
	[string]$Namespace = "ambience-dev",
	[string]$Selector = "app=ambience,component=edge",
	[string]$LocalDir = (Join-Path $PSScriptRoot "..\\cmd\\ambience\\web"),
	[string]$Container = "web-sync",
	[string]$RemoteDir = "/var/run/ambience-web-override",
	[int]$PollIntervalMs = 1000,
	[switch]$Once
)

$ErrorActionPreference = "Stop"
$script:TargetPod = $null

if ($PollIntervalMs -lt 200) {
	throw "PollIntervalMs must be at least 200."
}

$localRoot = (Get-Item -LiteralPath $LocalDir).FullName.TrimEnd("\", "/")

function Get-ReadyEdgePodName {
	$podList = kubectl get pod -n $Namespace -l $Selector -o json | ConvertFrom-Json
	$candidates = @($podList.items) | Where-Object {
		$_.status.phase -eq "Running" -and
		@($_.status.conditions | Where-Object { $_.type -eq "Ready" -and $_.status -eq "True" }).Count -gt 0
	} | Sort-Object { $_.metadata.creationTimestamp } -Descending

	if (-not $candidates -or $candidates.Count -eq 0) {
		throw "No ready edge pod found in namespace '$Namespace' for selector '$Selector'."
	}

	return $candidates[0].metadata.name
}

function Get-RelativeWebPath {
	param([string]$FullPath)

	$resolvedFullPath = (Get-Item -LiteralPath $FullPath).FullName
	if (-not $resolvedFullPath.StartsWith($localRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
		throw "Path '$resolvedFullPath' is outside local root '$localRoot'."
	}

	$relative = $resolvedFullPath.Substring($localRoot.Length).TrimStart("\", "/")
	return $relative.Replace("\", "/")
}

function Get-TargetPod {
	if ([string]::IsNullOrWhiteSpace($script:TargetPod)) {
		$script:TargetPod = Get-ReadyEdgePodName
	}

	return $script:TargetPod
}

function Get-RemotePath {
	param([string]$RelativePath)

	return ($RemoteDir.TrimEnd("/") + "/" + $RelativePath)
}

function Assert-DevLoopReady {
	$pod = Get-TargetPod
	kubectl exec $pod -n $Namespace -c $Container -- sh -c "mkdir -p '$RemoteDir' && test -d '$RemoteDir'" | Out-Null
	return $pod
}

function Sync-WebFile {
	param([string]$FullPath)

	if (-not (Test-Path -LiteralPath $FullPath -PathType Leaf)) {
		return
	}

	$pod = Get-TargetPod
	$relative = Get-RelativeWebPath -FullPath $FullPath
	$remotePath = Get-RemotePath -RelativePath $relative
	$remoteDirPath = [System.IO.Path]::GetDirectoryName($remotePath.Replace("/", [System.IO.Path]::DirectorySeparatorChar)).Replace("\", "/")
	if ([string]::IsNullOrWhiteSpace($remoteDirPath)) {
		$remoteDirPath = $RemoteDir
	}

	$copySource = $relative.Replace("/", "\")
	kubectl exec $pod -n $Namespace -c $Container -- sh -c "mkdir -p '$remoteDirPath'" | Out-Null
	Push-Location $localRoot
	try {
		kubectl cp $copySource "${Namespace}/${pod}:$remotePath" -c $Container | Out-Null
	} finally {
		Pop-Location
	}
	Write-Host ("synced {0} -> {1}" -f $relative, $pod)
}

function Remove-WebFile {
	param([string]$RelativePath)

	$pod = Get-TargetPod
	$remotePath = Get-RemotePath -RelativePath $RelativePath
	kubectl exec $pod -n $Namespace -c $Container -- sh -c "rm -f '$remotePath'" | Out-Null
	Write-Host ("removed {0} from {1}" -f $RelativePath, $pod)
}

function Get-WebState {
	$state = @{}
	Get-ChildItem -LiteralPath $localRoot -File -Recurse | ForEach-Object {
		$relative = Get-RelativeWebPath -FullPath $_.FullName
		$state[$relative] = "{0}:{1}" -f $_.LastWriteTimeUtc.Ticks, $_.Length
	}
	return $state
}

function Sync-AllWebFiles {
	$paths = Get-ChildItem -LiteralPath $localRoot -File -Recurse | Sort-Object FullName
	foreach ($path in $paths) {
		Sync-WebFile -FullPath $path.FullName
	}
}

$pod = Assert-DevLoopReady
Write-Host ("dev loop target pod: {0}" -f $pod)
Sync-AllWebFiles

if ($Once) {
	return
}

Write-Host ("watching {0} every {1}ms" -f $localRoot, $PollIntervalMs)
$known = Get-WebState

while ($true) {
	Start-Sleep -Milliseconds $PollIntervalMs

	try {
		$current = Get-WebState

		foreach ($relative in ($current.Keys | Sort-Object)) {
			if (-not $known.ContainsKey($relative) -or $known[$relative] -ne $current[$relative]) {
				$fullPath = Join-Path $localRoot ($relative -replace "/", "\")
				Sync-WebFile -FullPath $fullPath
			}
		}

		foreach ($relative in ($known.Keys | Sort-Object)) {
			if (-not $current.ContainsKey($relative)) {
				Remove-WebFile -RelativePath $relative
			}
		}

		$known = $current
	} catch {
		$script:TargetPod = $null
		Write-Warning $_
	}
}
