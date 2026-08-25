param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [Parameter(Mandatory = $true)]
    [string]$BaseRepo,

    [Parameter(Mandatory = $true)]
    [string]$CandidateRepo
)

$ErrorActionPreference = "Stop"
$baseRevision = "4c6b443e3abee2cb63953f53134cc7fd8f04593b"
$artifactRoot = [System.IO.Path]::GetFullPath($ArtifactDir)
$baseRoot = [System.IO.Path]::GetFullPath($BaseRepo)
$candidateRoot = [System.IO.Path]::GetFullPath($CandidateRepo)

if (Test-Path -LiteralPath $artifactRoot) {
    throw "official artifact directory already exists: $artifactRoot"
}
foreach ($repo in @($baseRoot, $candidateRoot)) {
    if (-not (Test-Path -LiteralPath (Join-Path $repo ".git"))) {
        throw "not a Git clone: $repo"
    }
    if (git -C $repo status --porcelain) {
        throw "official clone is dirty: $repo"
    }
    if (git -C $repo branch --show-current) {
        throw "official clone is not detached: $repo"
    }
}
if ((git -C $baseRoot rev-parse HEAD).Trim() -ne $baseRevision) {
    throw "baseline clone is not at the frozen revision"
}
$candidateRevision = (git -C $candidateRoot rev-parse HEAD).Trim()
if ($candidateRevision -notmatch "^[0-9a-f]{40}$" -or $candidateRevision -eq $baseRevision) {
    throw "invalid candidate revision: $candidateRevision"
}

$rawRoot = Join-Path $artifactRoot "raw"
$binaryRoot = Join-Path $rawRoot "binaries"
$reportRoot = Join-Path $rawRoot "reports"
$timingRoot = Join-Path $reportRoot "timing"
$profileRoot = Join-Path $rawRoot "profiles"
$provenanceRoot = Join-Path $rawRoot "provenance"
$scenarioRoot = Join-Path $rawRoot "scenarios"
foreach ($dir in @($binaryRoot, $reportRoot, $timingRoot, $profileRoot, $provenanceRoot, $scenarioRoot)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

$baseNormal = Join-Path $binaryRoot "base-normal.exe"
$baseProfiled = Join-Path $binaryRoot "base-searchprofile.exe"
$candidateNormal = Join-Path $binaryRoot "candidate-normal.exe"
$candidateProfiled = Join-Path $binaryRoot "candidate-searchprofile.exe"

Push-Location $baseRoot
try {
    & go build -buildvcs=true -o $baseNormal ./cmd/backpack-brawl-solver
    if ($LASTEXITCODE -ne 0) { throw "base normal build failed" }
    & go build -buildvcs=true -tags searchprofile -o $baseProfiled ./cmd/backpack-brawl-solver
    if ($LASTEXITCODE -ne 0) { throw "base searchprofile build failed" }
}
finally {
    Pop-Location
}
Push-Location $candidateRoot
try {
    & go build -buildvcs=true -o $candidateNormal ./cmd/backpack-brawl-solver
    if ($LASTEXITCODE -ne 0) { throw "candidate normal build failed" }
    & go build -buildvcs=true -tags searchprofile -o $candidateProfiled ./cmd/backpack-brawl-solver
    if ($LASTEXITCODE -ne 0) { throw "candidate searchprofile build failed" }
}
finally {
    Pop-Location
}

$metadataLines = @()
foreach ($entry in @(
    @{ Name = "base-normal"; Path = $baseNormal; Revision = $baseRevision },
    @{ Name = "base-searchprofile"; Path = $baseProfiled; Revision = $baseRevision },
    @{ Name = "candidate-normal"; Path = $candidateNormal; Revision = $candidateRevision },
    @{ Name = "candidate-searchprofile"; Path = $candidateProfiled; Revision = $candidateRevision }
)) {
    $metadata = (& go version -m $entry.Path | Out-String)
    if ($metadata -notmatch [regex]::Escape("vcs.revision=$($entry.Revision)")) {
        throw "$($entry.Name) binary revision mismatch"
    }
    if ($metadata -notmatch [regex]::Escape("vcs.modified=false")) {
        throw "$($entry.Name) binary is modified"
    }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $entry.Path).Hash.ToLowerInvariant()
    $metadataLines += "$($entry.Name)_sha256=$hash"
    $metadataLines += "$($entry.Name)_metadata:"
    $metadataLines += $metadata.TrimEnd()
}

$manifestPath = Join-Path $candidateRoot "benchmarks\suites\general-search-v2.json"
$lockPath = Join-Path $candidateRoot "benchmarks\suites\general-search-v2.lock"
$catalogPath = Join-Path $candidateRoot "data\catalog.json"
$generatorPath = Join-Path $candidateRoot "internal\benchmark\search_suite.go"
$systemLines = @(
    "R1I-G official collection provenance"
    "baseline_revision=$baseRevision"
    "candidate_revision=$candidateRevision"
    "base_status=clean_detached"
    "candidate_status=clean_detached"
    "validation_materialized=false"
    "public_holdout_materialized=false"
    "private_holdout_materialized=false"
    "go_version=$(& go version)"
    "goos=$(& go env GOOS)"
    "goarch=$(& go env GOARCH)"
    "timezone=$([System.TimeZoneInfo]::Local.Id)"
    "cpu=$((Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty Name).Trim())"
    "logical_processors=$((Get-CimInstance Win32_ComputerSystem).NumberOfLogicalProcessors)"
    "visible_memory_bytes=$((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory)"
    "catalog_sha256=$((Get-FileHash -Algorithm SHA256 -LiteralPath $catalogPath).Hash.ToLowerInvariant())"
    "suite_manifest_sha256=$((Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash.ToLowerInvariant())"
    "suite_lock_sha256=$((Get-FileHash -Algorithm SHA256 -LiteralPath $lockPath).Hash.ToLowerInvariant())"
    "generator_sha256=$((Get-FileHash -Algorithm SHA256 -LiteralPath $generatorPath).Hash.ToLowerInvariant())"
) + $metadataLines
[System.IO.File]::WriteAllLines((Join-Path $provenanceRoot "provenance.txt"), $systemLines, [System.Text.UTF8Encoding]::new($false))

Push-Location $candidateRoot
try {
    & $candidateNormal validate-catalog --catalog $catalogPath
    if ($LASTEXITCODE -ne 0) { throw "catalog validation failed" }
    & $candidateNormal verify-search-suite --manifest $manifestPath --catalog $catalogPath --lock $lockPath
    if ($LASTEXITCODE -ne 0) { throw "suite verification failed" }
    & $candidateNormal materialize-search-suite --manifest $manifestPath --catalog $catalogPath --lock $lockPath --roles development --out $scenarioRoot
    if ($LASTEXITCODE -ne 0) { throw "development materialization failed" }
}
finally {
    Pop-Location
}
$scenarioNames = @(Get-ChildItem -LiteralPath $scenarioRoot -Filter "gsv2-*.json" -File | Sort-Object Name | Select-Object -ExpandProperty BaseName)
$expectedNames = @(13..26 | ForEach-Object { "gsv2-{0:D3}" -f $_ })
if (($scenarioNames -join ",") -ne ($expectedNames -join ",")) {
    throw "materialized scenario set is not exactly gsv2-013..026"
}

Push-Location $candidateRoot
try {
    & go test ./internal/solver -run '^$' -bench '^(BenchmarkOutgoingPlacementLookup|BenchmarkOutgoingUpperPriorityCounts)$' -benchmem -count=10 2>&1 |
        Tee-Object -FilePath (Join-Path $rawRoot "microbench.txt")
    if ($LASTEXITCODE -ne 0) { throw "R1I-G microbenchmarks failed" }
}
finally {
    Pop-Location
}

function Invoke-R1IGBenchmark {
    param(
        [string]$Binary,
        [string]$Scenarios,
        [string]$Budgets,
        [string]$Variant,
        [string]$OutPath,
        [switch]$OperationProfile,
        [string]$CPUProfile = "",
        [string]$HeapProfile = ""
    )
    $arguments = @(
        "benchmark-scenarios",
        "--catalog", $catalogPath,
        "--dir", $scenarioRoot,
        "--scenarios", $Scenarios,
        "--budgets", $Budgets,
        "--repeat", "1",
        "--workers", "1",
        "--constellation-seed-variant", $Variant,
        "--out", $OutPath
    )
    if ($OperationProfile) { $arguments += "--operation-profile" }
    if ($CPUProfile) { $arguments += @("--cpu-profile", $CPUProfile) }
    if ($HeapProfile) { $arguments += @("--heap-profile", $HeapProfile) }
    & $Binary @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "solver run failed: $Binary $Scenarios $Budgets $Variant"
    }
}

Push-Location $candidateRoot
try {
    Invoke-R1IGBenchmark $baseNormal "gsv2-013" "250000" "general-search-v1" (Join-Path $reportRoot "smoke-base-normal.json")
    Invoke-R1IGBenchmark $candidateNormal "gsv2-013" "250000" "general-search-v1" (Join-Path $reportRoot "smoke-candidate-normal.json")
    Invoke-R1IGBenchmark $candidateProfiled "gsv2-013" "250000" "general-search-v1" (Join-Path $reportRoot "smoke-candidate-tagged-off.json")
    Invoke-R1IGBenchmark $baseProfiled "gsv2-013" "250000" "general-search-v1" (Join-Path $reportRoot "smoke-base-profiled.json") -OperationProfile
    Invoke-R1IGBenchmark $candidateProfiled "gsv2-013" "250000" "general-search-v1" (Join-Path $reportRoot "smoke-candidate-profiled.json") -OperationProfile

    $allScenarios = $expectedNames -join ","
    Invoke-R1IGBenchmark $baseProfiled $allScenarios "250000,1000000" "general-search-v1" (Join-Path $reportRoot "matrix-base-gsv1.json") -OperationProfile
    Invoke-R1IGBenchmark $candidateProfiled $allScenarios "250000,1000000" "general-search-v1" (Join-Path $reportRoot "matrix-candidate-gsv1.json") -OperationProfile
    Invoke-R1IGBenchmark $baseProfiled $allScenarios "1000000" "v4" (Join-Path $reportRoot "matrix-base-v4.json") -OperationProfile
    Invoke-R1IGBenchmark $candidateProfiled $allScenarios "1000000" "v4" (Join-Path $reportRoot "matrix-candidate-v4.json") -OperationProfile

    $timingScenarios = @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")
    foreach ($scenario in $timingScenarios) {
        foreach ($pair in 1..7) {
            $firstRole = if ($pair % 2 -eq 1) { "base" } else { "candidate" }
            $secondRole = if ($pair % 2 -eq 1) { "candidate" } else { "base" }
            foreach ($run in @(
                @{ Position = "a"; Role = $firstRole },
                @{ Position = "b"; Role = $secondRole }
            )) {
                $binary = if ($run.Role -eq "base") { $baseNormal } else { $candidateNormal }
                $filename = "timing-{0}-pair{1:D2}-{2}-{3}.json" -f $scenario, $pair, $run.Position, $run.Role
                Invoke-R1IGBenchmark $binary $scenario "1000000" "general-search-v1" (Join-Path $timingRoot $filename)
            }
        }
    }

    $profileScenarios = $timingScenarios -join ","
    Invoke-R1IGBenchmark $baseNormal $profileScenarios "1000000" "general-search-v1" (Join-Path $profileRoot "base-gsv1.json") -CPUProfile (Join-Path $profileRoot "base-gsv1.cpu.pprof") -HeapProfile (Join-Path $profileRoot "base-gsv1.heap.pprof")
    Invoke-R1IGBenchmark $candidateNormal $profileScenarios "1000000" "general-search-v1" (Join-Path $profileRoot "candidate-gsv1.json") -CPUProfile (Join-Path $profileRoot "candidate-gsv1.cpu.pprof") -HeapProfile (Join-Path $profileRoot "candidate-gsv1.heap.pprof")
    Invoke-R1IGBenchmark $baseNormal $profileScenarios "1000000" "v4" (Join-Path $profileRoot "base-v4.json") -CPUProfile (Join-Path $profileRoot "base-v4.cpu.pprof")
    Invoke-R1IGBenchmark $candidateNormal $profileScenarios "1000000" "v4" (Join-Path $profileRoot "candidate-v4.json") -CPUProfile (Join-Path $profileRoot "candidate-v4.cpu.pprof")
}
finally {
    Pop-Location
}

$freezeScript = Join-Path $candidateRoot "benchmarks\profiling\r1ig-freeze.ps1"
& $freezeScript -ArtifactDir $artifactRoot
if ($LASTEXITCODE -ne 0) {
    throw "R1I-G freeze failed"
}
Write-Output "R1I-G official collection and immediate freeze complete. Do not run another solver against this bundle."
