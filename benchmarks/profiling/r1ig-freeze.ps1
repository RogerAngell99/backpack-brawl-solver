param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir
)

$ErrorActionPreference = "Stop"
$artifactRoot = [System.IO.Path]::GetFullPath($ArtifactDir)
$rawRoot = Join-Path $artifactRoot "raw"
if (-not (Test-Path -LiteralPath $rawRoot -PathType Container)) {
    throw "raw artifact directory does not exist: $rawRoot"
}

$freezeRoot = Join-Path $artifactRoot "freeze"
New-Item -ItemType Directory -Force -Path $freezeRoot | Out-Null
$manifestPath = Join-Path $freezeRoot "SHA256SUMS.txt"
$manifestHashPath = Join-Path $freezeRoot "manifest-sha256.txt"
$recordPath = Join-Path $freezeRoot "freeze-record.txt"

$files = @(Get-ChildItem -LiteralPath $rawRoot -File -Recurse | Sort-Object FullName)
$totalBytes = ($files | Measure-Object -Property Length -Sum).Sum
if ($null -eq $totalBytes) {
    $totalBytes = 0
}
$lines = foreach ($file in $files) {
    $relative = [System.IO.Path]::GetRelativePath($rawRoot, $file.FullName).Replace("\", "/")
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant()
    "$hash  $relative"
}
[System.IO.File]::WriteAllLines($manifestPath, $lines, [System.Text.UTF8Encoding]::new($false))
$manifestHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllLines($manifestHashPath, @("$manifestHash  SHA256SUMS.txt"), [System.Text.UTF8Encoding]::new($false))

$record = @(
    "R1I-G raw artifact freeze"
    "raw_file_count=$($files.Count)"
    "raw_total_bytes=$totalBytes"
    "manifest_sha256=$manifestHash"
    "read_only=true"
    "hash_revalidation=PASS"
    "post_freeze_solver_runs=0"
)
[System.IO.File]::WriteAllLines($recordPath, $record, [System.Text.UTF8Encoding]::new($false))

foreach ($file in $files) {
    $file.IsReadOnly = $true
}
(Get-Item -LiteralPath $manifestPath).IsReadOnly = $true
(Get-Item -LiteralPath $manifestHashPath).IsReadOnly = $true
(Get-Item -LiteralPath $recordPath).IsReadOnly = $true

foreach ($line in $lines) {
    $expected = $line.Substring(0, 64)
    $relative = $line.Substring(66).Replace("/", [System.IO.Path]::DirectorySeparatorChar)
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $rawRoot $relative)).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw "raw artifact hash changed during freeze: $relative"
    }
}
$actualManifestHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash.ToLowerInvariant()
if ($actualManifestHash -ne $manifestHash) {
    throw "manifest hash changed during freeze"
}

Write-Output "R1I-G raw artifacts frozen: $($files.Count) files, $totalBytes bytes"
Write-Output "manifest_sha256=$manifestHash"
Write-Output "post_freeze_solver_runs=0"
