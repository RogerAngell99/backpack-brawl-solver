param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [string]$OutputDir = ""
)

$ErrorActionPreference = "Stop"
$artifactRoot = [System.IO.Path]::GetFullPath($ArtifactDir)
$rawRoot = Join-Path $artifactRoot "raw"
if (-not $OutputDir) {
    $OutputDir = Join-Path $artifactRoot "profile-review"
}
$reviewRoot = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Force -Path $reviewRoot | Out-Null

function Invoke-PProfExtract {
    param(
        [string]$OutputName,
        [string]$Binary,
        [string]$Profile,
        [string[]]$Arguments
    )
    $outputPath = Join-Path $reviewRoot $OutputName
    $output = & go tool pprof @Arguments $Binary $Profile 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "pprof extraction failed: $OutputName"
    }
    [System.IO.File]::WriteAllLines($outputPath, @($output), [System.Text.UTF8Encoding]::new($false))
}

$profiles = @(
    @{ Role = "base"; Binary = Join-Path $rawRoot "binaries\base-normal.exe" },
    @{ Role = "candidate"; Binary = Join-Path $rawRoot "binaries\candidate-normal.exe" }
)
foreach ($entry in $profiles) {
    $role = $entry.Role
    $binary = $entry.Binary
    $gsv1CPU = Join-Path $rawRoot "profiles\$role-gsv1.cpu.pprof"
    $v4CPU = Join-Path $rawRoot "profiles\$role-v4.cpu.pprof"
    $heap = Join-Path $rawRoot "profiles\$role-gsv1.heap.pprof"

    Invoke-PProfExtract "$role-gsv1-cpu-top.txt" $binary $gsv1CPU @("-top")
    Invoke-PProfExtract "$role-gsv1-cpu-top-cum.txt" $binary $gsv1CPU @("-top", "-cum")
    Invoke-PProfExtract "$role-gsv1-outgoing-source.txt" $binary $gsv1CPU @("-list", "upperPriorityCounts")
    Invoke-PProfExtract "$role-gsv1-index-source.txt" $binary $gsv1CPU @("-list", "buildOutgoingPlacementIndex")
    Invoke-PProfExtract "$role-gsv1-outgoing-callers.txt" $binary $gsv1CPU @("-peek", "upperPriorityCounts|placementByInstanceID|buildOutgoingPlacementIndex")
    Invoke-PProfExtract "$role-v4-cpu-top.txt" $binary $v4CPU @("-top")
    Invoke-PProfExtract "$role-v4-outgoing-source.txt" $binary $v4CPU @("-list", "upperPriorityCounts")
    Invoke-PProfExtract "$role-v4-index-source.txt" $binary $v4CPU @("-list", "buildOutgoingPlacementIndex")
    Invoke-PProfExtract "$role-heap-alloc-space.txt" $binary $heap @("-sample_index=alloc_space", "-top")
    Invoke-PProfExtract "$role-heap-alloc-objects.txt" $binary $heap @("-sample_index=alloc_objects", "-top")
    Invoke-PProfExtract "$role-heap-inuse-space.txt" $binary $heap @("-sample_index=inuse_space", "-top")
}

Write-Output "Wrote R1I-G causal profile extracts to $reviewRoot"
