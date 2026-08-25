[CmdletBinding()]
param(
    [string]$ArtifactDir,
    [switch]$Preflight
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($Preflight) {
    Write-Output "status=PASS"
    Write-Output "raw_exclusions=derived,review,review-input"
    Write-Output "post_freeze_solver_runs=0"
    return
}
if ([string]::IsNullOrWhiteSpace($ArtifactDir)) { throw "ArtifactDir is required outside preflight" }

$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir).TrimEnd('\')
$ProvenanceDir = Join-Path $ArtifactDir "provenance"
$ManifestPath = Join-Path $ProvenanceDir "RAW-SHA256SUMS.txt"
$ManifestHashPath = Join-Path $ProvenanceDir "RAW-SHA256SUMS.sha256"
$FreezeRecordPath = Join-Path $ProvenanceDir "freeze-record.txt"
$Utf8NoBom = New-Object Text.UTF8Encoding($false)

foreach ($Relative in @(
    "binaries/r1ih-normal.exe",
    "binaries/r1ih-searchprofile.exe",
    "operations/r1ih-gsv1.json",
    "operations/r1ih-v4.json",
    "profiles/r1ih-gsv1.cpu.pprof",
    "profiles/r1ih-gsv1.heap.pprof",
    "profiles/r1ih-v4.cpu.pprof",
    "provenance/official-collection-complete.txt"
)) {
    $Path = Join-Path $ArtifactDir $Relative
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "required final raw artifact missing: $Relative" }
}
foreach ($Scenario in @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")) {
    $Path = Join-Path $ArtifactDir "profiles/per-case/$Scenario.cpu.pprof"
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "per-case CPU profile missing: $Scenario" }
}

New-Item -ItemType Directory -Force -Path $ProvenanceDir | Out-Null
$Excluded = @($ManifestPath, $ManifestHashPath, $FreezeRecordPath)
$Payloads = @(
    Get-ChildItem -LiteralPath $ArtifactDir -Recurse -File |
        Where-Object {
            $_.FullName -notin $Excluded -and
            $_.FullName -notlike (Join-Path $ArtifactDir "derived\*") -and
            $_.FullName -notlike (Join-Path $ArtifactDir "review\*") -and
            $_.FullName -notlike (Join-Path $ArtifactDir "review-input\*")
        } |
        Sort-Object FullName
)
if ($Payloads.Count -eq 0) { throw "raw payload set is empty" }

$RootPrefix = $ArtifactDir + '\'
$ManifestLines = [Collections.Generic.List[string]]::new()
[int64]$TotalBytes = 0
foreach ($Payload in $Payloads) {
    if (-not $Payload.FullName.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw "payload escaped artifact root: $($Payload.FullName)" }
    $Relative = $Payload.FullName.Substring($RootPrefix.Length).Replace('\', '/')
    $Hash = (Get-FileHash -LiteralPath $Payload.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    $ManifestLines.Add("$Hash  $Relative")
    $TotalBytes += $Payload.Length
}
[IO.File]::WriteAllText($ManifestPath, (($ManifestLines -join "`n") + "`n"), $Utf8NoBom)
$ManifestHash = (Get-FileHash -LiteralPath $ManifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($ManifestHashPath, "$ManifestHash  provenance/RAW-SHA256SUMS.txt`n", $Utf8NoBom)

foreach ($Payload in $Payloads) { $Payload.IsReadOnly = $true }
(Get-Item -LiteralPath $ManifestPath).IsReadOnly = $true
(Get-Item -LiteralPath $ManifestHashPath).IsReadOnly = $true

foreach ($Line in $ManifestLines) {
    $Parts = $Line -split '  ', 2
    $Path = Join-Path $ArtifactDir ($Parts[1].Replace('/', '\'))
    $Actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($Actual -ne $Parts[0]) { throw "post-freeze hash mismatch: $($Parts[1])" }
    if (-not (Get-Item -LiteralPath $Path).IsReadOnly) { throw "payload is not read-only: $($Parts[1])" }
}
if ((Get-FileHash -LiteralPath $ManifestPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $ManifestHash) { throw "manifest hash changed during revalidation" }

$Record = @(
    "R1I-H raw artifact freeze",
    "artifact_dir=$ArtifactDir",
    "raw_file_count=$($Payloads.Count)",
    "raw_total_bytes=$TotalBytes",
    "raw_manifest_sha256=$ManifestHash",
    "raw_manifest_revalidation=PASS",
    "raw_read_only_revalidation=PASS",
    "post_freeze_solver_runs=0",
    "frozen_at=$([DateTimeOffset]::Now.ToString('o'))"
)
[IO.File]::WriteAllText($FreezeRecordPath, (($Record -join "`n") + "`n"), $Utf8NoBom)
(Get-Item -LiteralPath $FreezeRecordPath).IsReadOnly = $true
Write-Output ($Record -join "`n")
