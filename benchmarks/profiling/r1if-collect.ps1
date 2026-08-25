[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$RepoDir,

    [Parameter(Mandatory = $true)]
    [string]$WebRepoDir,

    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [Parameter(Mandatory = $true)]
    [string]$ProtocolCommit
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ExpectedSHA = "9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b"
$RepoDir = [IO.Path]::GetFullPath($RepoDir)
$WebRepoDir = [IO.Path]::GetFullPath($WebRepoDir)
$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir)
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ExpectedCases = 13..26 | ForEach-Object { "gsv2-$('{0:D3}' -f $_)" }
$ProfileCases = @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")
$AllScenarios = $ExpectedCases -join ','
$ProfileScenarios = $ProfileCases -join ','

function Write-Utf8Text([string]$Path, [string]$Value) {
    $Parent = Split-Path -Parent $Path
    if ($Parent) { New-Item -ItemType Directory -Force -Path $Parent | Out-Null }
    [IO.File]::WriteAllText($Path, $Value, $Utf8NoBom)
}

function Invoke-Logged([string]$Command, [string[]]$Arguments, [string]$WorkingDirectory, [string]$LogPath) {
    Push-Location $WorkingDirectory
    try {
        [string[]]$Output = & $Command @Arguments 2>&1 | ForEach-Object { [string]$_ }
        $ExitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    Write-Utf8Text $LogPath (((@("command=$Command $($Arguments -join ' ')", "exit_code=$ExitCode") + $Output) -join "`n") + "`n")
    if ($ExitCode -ne 0) { throw "command failed ($ExitCode): $Command $($Arguments -join ' ')" }
    return ,$Output
}

function Assert-DetachedCleanAt([string]$Path, [string]$Label) {
    $Head = (& git -C $Path rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $Head -ne $ExpectedSHA) { throw "$Label HEAD=$Head, expected $ExpectedSHA" }
    [string[]]$Status = @(& git -C $Path status --porcelain=v1)
    if ($LASTEXITCODE -ne 0 -or $Status.Count -ne 0) { throw "$Label must be clean" }
    [string]$Branch = ((& git -C $Path branch --show-current) -join "").Trim()
    if ($Branch) { throw "$Label must be detached, found branch $Branch" }
}

if (Test-Path -LiteralPath $ArtifactDir) {
    $Existing = @(Get-ChildItem -LiteralPath $ArtifactDir -Force)
    if ($Existing.Count -ne 0) { throw "artifact directory must be new or empty: $ArtifactDir" }
}
New-Item -ItemType Directory -Force -Path $ArtifactDir | Out-Null
foreach ($Directory in @("binaries", "operations", "profiles", "profiles\per-case", "provenance", "scenarios", "smoke")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $ArtifactDir $Directory) | Out-Null
}

Assert-DetachedCleanAt $RepoDir "measurement clone"
Assert-DetachedCleanAt $WebRepoDir "Web/WASM clone"
if ($ProtocolCommit -notmatch '^[0-9a-f]{40}$') { throw "invalid protocol commit: $ProtocolCommit" }

$ProvenanceDir = Join-Path $ArtifactDir "provenance"
$GateDir = Join-Path $ProvenanceDir "gates"
New-Item -ItemType Directory -Force -Path $GateDir | Out-Null

[void](Invoke-Logged "git" @("diff", "--check") $RepoDir (Join-Path $GateDir "git-diff-check.txt"))
[void](Invoke-Logged "go" @("test", "./...") $RepoDir (Join-Path $GateDir "go-test.txt"))
[void](Invoke-Logged "go" @("test", "-tags", "searchprofile", "./...") $RepoDir (Join-Path $GateDir "go-test-searchprofile.txt"))
[void](Invoke-Logged "go" @("test", "-race", "./internal/solver/...") $RepoDir (Join-Path $GateDir "go-test-race-solver.txt"))
[void](Invoke-Logged "go" @("test", "-race", "-tags", "searchprofile", "./internal/solver/...") $RepoDir (Join-Path $GateDir "go-test-race-searchprofile-solver.txt"))

$SnapshotDir = Join-Path $ProvenanceDir "semantic-snapshot"
New-Item -ItemType Directory -Force -Path $SnapshotDir | Out-Null
$OldSnapshotEnv = $env:R1I_SEMANTIC_SNAPSHOT
try {
    $env:R1I_SEMANTIC_SNAPSHOT = Join-Path $SnapshotDir "normal.json"
    [void](Invoke-Logged "go" @("test", "./internal/solver", "-run", "^TestR1ICrossBuildSemanticSnapshot$", "-count=1") $RepoDir (Join-Path $GateDir "semantic-normal.txt"))
    $env:R1I_SEMANTIC_SNAPSHOT = Join-Path $SnapshotDir "tagged-off.json"
    [void](Invoke-Logged "go" @("test", "-tags", "searchprofile", "./internal/solver", "-run", "^TestR1ICrossBuildSemanticSnapshot$", "-count=1") $RepoDir (Join-Path $GateDir "semantic-tagged-off.txt"))
} finally {
    $env:R1I_SEMANTIC_SNAPSHOT = $OldSnapshotEnv
}
$NormalSnapshotHash = (Get-FileHash -LiteralPath (Join-Path $SnapshotDir "normal.json") -Algorithm SHA256).Hash
$TaggedSnapshotHash = (Get-FileHash -LiteralPath (Join-Path $SnapshotDir "tagged-off.json") -Algorithm SHA256).Hash
if ($NormalSnapshotHash -ne $TaggedSnapshotHash) { throw "normal/tagged-off semantic snapshots differ" }
Write-Utf8Text (Join-Path $GateDir "semantic-comparison.txt") "normal_sha256=$NormalSnapshotHash`ntagged_off_sha256=$TaggedSnapshotHash`ncomparison=PASS`n"

[void](Invoke-Logged "npm" @("ci") (Join-Path $WebRepoDir "web") (Join-Path $GateDir "npm-ci.txt"))
[void](Invoke-Logged "npm" @("run", "build") (Join-Path $WebRepoDir "web") (Join-Path $GateDir "web-wasm-build.txt"))

$NormalBinary = Join-Path $ArtifactDir "binaries\solver.exe"
$ProfileBinary = Join-Path $ArtifactDir "binaries\solver-searchprofile.exe"
[void](Invoke-Logged "go" @("build", "-buildvcs=true", "-o", $NormalBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $ProvenanceDir "build-normal.txt"))
[void](Invoke-Logged "go" @("build", "-buildvcs=true", "-tags", "searchprofile", "-o", $ProfileBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $ProvenanceDir "build-searchprofile.txt"))

$NormalMetadata = Invoke-Logged "go" @("version", "-m", $NormalBinary) $RepoDir (Join-Path $ProvenanceDir "solver-normal.metadata.txt")
$ProfileMetadata = Invoke-Logged "go" @("version", "-m", $ProfileBinary) $RepoDir (Join-Path $ProvenanceDir "solver-searchprofile.metadata.txt")
foreach ($Metadata in @($NormalMetadata, $ProfileMetadata)) {
    $Text = $Metadata -join "`n"
    if ($Text -notmatch "vcs.revision=$ExpectedSHA") { throw "binary revision mismatch" }
    if ($Text -notmatch "vcs.modified=false") { throw "binary is marked modified" }
}

[void](Invoke-Logged $NormalBinary @("validate-catalog", "--catalog", "data/catalog.json") $RepoDir (Join-Path $GateDir "catalog-validation.txt"))
[void](Invoke-Logged $NormalBinary @("verify-search-suite", "--manifest", "benchmarks/suites/general-search-v2.json", "--catalog", "data/catalog.json", "--lock", "benchmarks/suites/general-search-v2.lock") $RepoDir (Join-Path $ProvenanceDir "suite-verification.txt"))
[void](Invoke-Logged $NormalBinary @("materialize-search-suite", "--manifest", "benchmarks/suites/general-search-v2.json", "--catalog", "data/catalog.json", "--lock", "benchmarks/suites/general-search-v2.lock", "--roles", "development", "--out", (Join-Path $ArtifactDir "scenarios")) $RepoDir (Join-Path $ProvenanceDir "materialization-command.txt"))

$Materialized = @(Get-ChildItem -LiteralPath (Join-Path $ArtifactDir "scenarios") -File -Filter "*.json" | Sort-Object Name)
if ($Materialized.Count -ne 14) { throw "materialized file count=$($Materialized.Count), expected 14" }
$ActualNames = $Materialized.Name
$ExpectedNames = $ExpectedCases | ForEach-Object { "$_.json" }
if (($ActualNames -join ',') -ne ($ExpectedNames -join ',')) { throw "development materialization contained unexpected cases" }
Write-Utf8Text (Join-Path $ProvenanceDir "materialization.txt") ((@(
    "roles=development",
    "case_count=14",
    "cases=$($ExpectedCases -join ',')",
    "validation_materialized=false",
    "public_holdout_materialized=false",
    "private_holdout_materialized=false"
) -join "`n") + "`n")

$CatalogHash = (Get-FileHash -LiteralPath (Join-Path $RepoDir "data\catalog.json") -Algorithm SHA256).Hash.ToLowerInvariant()
$ManifestHash = (Get-FileHash -LiteralPath (Join-Path $RepoDir "benchmarks\suites\general-search-v2.json") -Algorithm SHA256).Hash.ToLowerInvariant()
$LockHash = (Get-FileHash -LiteralPath (Join-Path $RepoDir "benchmarks\suites\general-search-v2.lock") -Algorithm SHA256).Hash.ToLowerInvariant()
$GeneratorRegistryHash = (Get-FileHash -LiteralPath (Join-Path $RepoDir "internal\benchmark\suite_generator.go") -Algorithm SHA256).Hash.ToLowerInvariant()
$GeneratorV2Hash = (Get-FileHash -LiteralPath (Join-Path $RepoDir "internal\benchmark\suite_generator_v2.go") -Algorithm SHA256).Hash.ToLowerInvariant()
$NormalHash = (Get-FileHash -LiteralPath $NormalBinary -Algorithm SHA256).Hash.ToLowerInvariant()
$ProfileHash = (Get-FileHash -LiteralPath $ProfileBinary -Algorithm SHA256).Hash.ToLowerInvariant()
$GoVersion = (& go version) -join "`n"
$GoEnv = (& go env GOOS GOARCH) -join ','
$OS = Get-CimInstance Win32_OperatingSystem
$CPU = Get-CimInstance Win32_Processor | Select-Object -First 1
$Computer = Get-CimInstance Win32_ComputerSystem
$TimeZone = Get-TimeZone
$GitStatus = (& git -C $RepoDir status --porcelain=v1) -join "`n"
if ($GitStatus) { throw "measurement clone became dirty before collection" }

$Provenance = @(
    "R1I-F official collection provenance",
    "frozen_sha=$ExpectedSHA",
    "protocol_commit=$ProtocolCommit",
    "git_status=clean",
    "detached_head=true",
    "normal_binary_sha256=$NormalHash",
    "searchprofile_binary_sha256=$ProfileHash",
    "go_version=$GoVersion",
    "goos_goarch=$GoEnv",
    "os_caption=$($OS.Caption)",
    "os_version=$($OS.Version)",
    "os_build=$($OS.BuildNumber)",
    "cpu=$($CPU.Name)",
    "logical_processors=$($Computer.NumberOfLogicalProcessors)",
    "visible_ram_bytes=$($Computer.TotalPhysicalMemory)",
    "timezone=$($TimeZone.Id)",
    "catalog_sha256=$CatalogHash",
    "verified_catalog_digest=$CatalogHash",
    "suite_manifest_sha256=$ManifestHash",
    "suite_lock_sha256=$LockHash",
    "generator_registry_sha256=$GeneratorRegistryHash",
    "generator_v2_sha256=$GeneratorV2Hash",
    "operation_matrix_gsv1=cases:gsv2-013..026 budgets:250000,1000000 repeat:1 workers:1 operation_profile:true diagnostic:false",
    "operation_matrix_v4=cases:gsv2-013..026 budgets:1000000 repeat:1 workers:1 operation_profile:true diagnostic:false",
    "cpu_slice=$($ProfileCases -join ',') budget:1000000 repeat:1 workers:1 operation_profile:false diagnostic:false",
    "validation_materialized=false",
    "public_holdout_materialized=false",
    "private_holdout_materialized=false"
)
Write-Utf8Text (Join-Path $ProvenanceDir "provenance.txt") (($Provenance -join "`n") + "`n")

$ScenarioDir = Join-Path $ArtifactDir "scenarios"
[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--out", (Join-Path $ArtifactDir "smoke\r1if-smoke-normal.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-normal.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--out", (Join-Path $ArtifactDir "smoke\r1if-smoke-tagged-off.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-tagged-off.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--operation-profile", "--out", (Join-Path $ArtifactDir "smoke\r1if-smoke-tagged-on.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-tagged-on.txt"))

[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $AllScenarios, "--budgets", "250000,1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--operation-profile", "--out", (Join-Path $ArtifactDir "operations\r1if-gsv1.json")) $RepoDir (Join-Path $ProvenanceDir "operations-gsv1.txt"))
[void](Invoke-Logged $ProfileBinary @("summarize-operation-profile", "--out", (Join-Path $ArtifactDir "operations\r1if-gsv1-summary.json"), (Join-Path $ArtifactDir "operations\r1if-gsv1.json")) $RepoDir (Join-Path $ProvenanceDir "operations-gsv1-summary.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $AllScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "v4", "--operation-profile", "--out", (Join-Path $ArtifactDir "operations\r1if-v4.json")) $RepoDir (Join-Path $ProvenanceDir "operations-v4.txt"))
[void](Invoke-Logged $ProfileBinary @("summarize-operation-profile", "--out", (Join-Path $ArtifactDir "operations\r1if-v4-summary.json"), (Join-Path $ArtifactDir "operations\r1if-v4.json")) $RepoDir (Join-Path $ProvenanceDir "operations-v4-summary.txt"))

[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $ProfileScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--cpu-profile", (Join-Path $ArtifactDir "profiles\r1if-gsv1.cpu.pprof"), "--heap-profile", (Join-Path $ArtifactDir "profiles\r1if-gsv1.heap.pprof"), "--out", (Join-Path $ArtifactDir "profiles\r1if-gsv1-profile.json")) $RepoDir (Join-Path $ProvenanceDir "profile-gsv1-combined.txt"))
foreach ($Scenario in $ProfileCases) {
    [void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $Scenario, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--cpu-profile", (Join-Path $ArtifactDir "profiles\per-case\$Scenario.cpu.pprof"), "--out", (Join-Path $ArtifactDir "profiles\per-case\$Scenario.json")) $RepoDir (Join-Path $ProvenanceDir "profile-$Scenario.txt"))
}
[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $ProfileScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "v4", "--cpu-profile", (Join-Path $ArtifactDir "profiles\r1if-v4.cpu.pprof"), "--out", (Join-Path $ArtifactDir "profiles\r1if-v4-profile.json")) $RepoDir (Join-Path $ProvenanceDir "profile-v4-combined.txt"))

Write-Output "R1I-F official solver collection complete. Run r1if-freeze.ps1 before any further solver command."
