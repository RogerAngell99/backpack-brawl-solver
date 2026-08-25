[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("Preflight", "Official")]
    [string]$Mode,

    [Parameter(Mandatory = $true)]
    [string]$RepoDir,

    [Parameter(Mandatory = $true)]
    [string]$WebRepoDir,

    [Parameter(Mandatory = $true)]
    [string]$ToolingRepoDir,

    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [string]$ProtocolCommit,

    [string]$PreflightRecord
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ExpectedSHA = "0cac463a79238ecaea9d95af33468cc04dd5809b"
$RepoDir = [IO.Path]::GetFullPath($RepoDir)
$WebRepoDir = [IO.Path]::GetFullPath($WebRepoDir)
$ToolingRepoDir = [IO.Path]::GetFullPath($ToolingRepoDir)
$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir)
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ExpectedCases = @(13..26 | ForEach-Object { "gsv2-$('{0:D3}' -f $_)" })
$ProfileCases = @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")
$AllScenarios = $ExpectedCases -join ','
$ProfileScenarios = $ProfileCases -join ','
$ToolFiles = [ordered]@{
    protocol = "benchmarks/profiling/r1ih-protocol.md"
    collector = "benchmarks/profiling/r1ih-collect.ps1"
    extractor = "benchmarks/profiling/r1ih-profile-extract.ps1"
    analysis = "benchmarks/profiling/r1ih-analysis.mjs"
    freeze_script = "benchmarks/profiling/r1ih-freeze.ps1"
}

function Write-Utf8Text([string]$Path, [string]$Value) {
    $Parent = Split-Path -Parent $Path
    if ($Parent) { New-Item -ItemType Directory -Force -Path $Parent | Out-Null }
    [IO.File]::WriteAllText($Path, $Value, $Utf8NoBom)
}

function Assert-File([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "required file missing: $Path" }
}

function New-EmptyArtifactDir([string]$Path) {
    if (Test-Path -LiteralPath $Path) {
        $Existing = @(Get-ChildItem -LiteralPath $Path -Force)
        if ($Existing.Count -ne 0) { throw "artifact directory must be new or empty: $Path" }
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Invoke-Logged(
    [string]$Command,
    [string[]]$Arguments,
    [string]$WorkingDirectory,
    [string]$LogPath,
    [int[]]$AllowedExitCodes = @(0)
) {
    Push-Location $WorkingDirectory
    try {
        [string[]]$Output = @(& $Command @Arguments 2>&1 | ForEach-Object { [string]$_ })
        $ExitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    Write-Utf8Text $LogPath (((@("command=$Command $($Arguments -join ' ')", "exit_code=$ExitCode") + $Output) -join "`n") + "`n")
    if ($ExitCode -notin $AllowedExitCodes) { throw "command failed ($ExitCode): $Command $($Arguments -join ' ')" }
    return $Output
}

function Assert-DetachedCleanAt([string]$Path, [string]$Label) {
    $Head = ((& git -C $Path rev-parse HEAD) -join "").Trim()
    if ($LASTEXITCODE -ne 0 -or $Head -ne $ExpectedSHA) { throw "$Label HEAD=$Head, expected $ExpectedSHA" }
    [string[]]$Status = @(& git -C $Path status --porcelain=v1)
    if ($LASTEXITCODE -ne 0 -or $Status.Count -ne 0) { throw "$Label must be clean" }
    $Branch = ((& git -C $Path branch --show-current) -join "").Trim()
    if ($Branch) { throw "$Label must be detached, found branch $Branch" }
}

function Assert-ToolingFiles {
    foreach ($Relative in $ToolFiles.Values) { Assert-File (Join-Path $ToolingRepoDir $Relative) }
}

function Get-ToolHashes {
    $Hashes = [ordered]@{}
    foreach ($Entry in $ToolFiles.GetEnumerator()) {
        $Path = Join-Path $ToolingRepoDir $Entry.Value
        $Hashes[$Entry.Key] = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    return $Hashes
}

function Get-GitBlob([string]$Repository, [string]$Commit, [string]$RelativePath) {
    $Spec = "${Commit}:$($RelativePath.Replace('\', '/'))"
    $Blob = ((& git -C $Repository rev-parse $Spec) -join "").Trim()
    if ($LASTEXITCODE -ne 0 -or $Blob -notmatch '^[0-9a-f]{40}$') { throw "cannot resolve Git blob $Spec" }
    return $Blob
}

function Read-KeyValues([string]$Path) {
    Assert-File $Path
    $Values = @{}
    foreach ($Line in [IO.File]::ReadAllLines($Path)) {
        if ($Line -match '^(?<key>[A-Za-z0-9_]+)=(?<value>.*)$') { $Values[$Matches.key] = $Matches.value }
    }
    return $Values
}

function Assert-PowerShellSyntax([string]$Path) {
    $Tokens = $null
    $Errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile($Path, [ref]$Tokens, [ref]$Errors)
    if (@($Errors).Count -ne 0) {
        throw "PowerShell syntax failure in $Path`: $((@($Errors) | ForEach-Object Message) -join '; ')"
    }
}

function Assert-GofmtClean([string]$LogDir) {
    [string[]]$Output = @(Invoke-Logged "gofmt" @("-l", ".") $RepoDir (Join-Path $LogDir "gofmt.txt"))
    if ($Output.Count -ne 0) { throw "gofmt -l reported files: $($Output -join ', ')" }
}

function Invoke-SemanticSnapshotGate([string]$RootDir, [string]$LogDir) {
    $SnapshotDir = Join-Path $RootDir "semantic-snapshot"
    New-Item -ItemType Directory -Force -Path $SnapshotDir | Out-Null
    $OldSnapshotEnv = $env:R1I_SEMANTIC_SNAPSHOT
    try {
        $env:R1I_SEMANTIC_SNAPSHOT = Join-Path $SnapshotDir "normal.json"
        [void](Invoke-Logged "go" @("test", "./internal/solver", "-run", "^TestR1ICrossBuildSemanticSnapshot$", "-count=1") $RepoDir (Join-Path $LogDir "semantic-normal.txt"))
        $env:R1I_SEMANTIC_SNAPSHOT = Join-Path $SnapshotDir "tagged-off.json"
        [void](Invoke-Logged "go" @("test", "-tags", "searchprofile", "./internal/solver", "-run", "^TestR1ICrossBuildSemanticSnapshot$", "-count=1") $RepoDir (Join-Path $LogDir "semantic-tagged-off.txt"))
    } finally {
        $env:R1I_SEMANTIC_SNAPSHOT = $OldSnapshotEnv
    }
    $NormalHash = (Get-FileHash -LiteralPath (Join-Path $SnapshotDir "normal.json") -Algorithm SHA256).Hash.ToLowerInvariant()
    $TaggedHash = (Get-FileHash -LiteralPath (Join-Path $SnapshotDir "tagged-off.json") -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($NormalHash -ne $TaggedHash) { throw "normal/searchprofile-OFF semantic snapshots differ" }
    Write-Utf8Text (Join-Path $LogDir "semantic-comparison.txt") "normal_sha256=$NormalHash`ntagged_off_sha256=$TaggedHash`ncomparison=PASS`n"
}

function Invoke-RepositoryGates([string]$RootDir, [switch]$IncludeWeb) {
    $GateDir = Join-Path $RootDir "gates"
    New-Item -ItemType Directory -Force -Path $GateDir | Out-Null
    Assert-GofmtClean $GateDir
    [void](Invoke-Logged "git" @("diff", "--check") $RepoDir (Join-Path $GateDir "git-diff-check.txt"))
    [void](Invoke-Logged "go" @("test", "./...") $RepoDir (Join-Path $GateDir "go-test.txt"))
    [void](Invoke-Logged "go" @("test", "-tags", "searchprofile", "./...") $RepoDir (Join-Path $GateDir "go-test-searchprofile.txt"))
    [void](Invoke-Logged "go" @("test", "-race", "./internal/solver/...") $RepoDir (Join-Path $GateDir "go-test-race-solver.txt"))
    [void](Invoke-Logged "go" @("test", "-race", "-tags", "searchprofile", "./internal/solver/...") $RepoDir (Join-Path $GateDir "go-test-race-searchprofile-solver.txt"))
    Invoke-SemanticSnapshotGate $RootDir $GateDir
    if ($IncludeWeb) {
        [void](Invoke-Logged "npm" @("ci") (Join-Path $WebRepoDir "web") (Join-Path $GateDir "npm-ci.txt"))
        [void](Invoke-Logged "npm" @("run", "build") (Join-Path $WebRepoDir "web") (Join-Path $GateDir "web-wasm-build.txt"))
    }
}

function Assert-BinaryMetadata([string]$Binary, [string]$Label, [string]$LogPath) {
    [string[]]$Metadata = @(Invoke-Logged "go" @("version", "-m", $Binary) $RepoDir $LogPath)
    $Text = $Metadata -join "`n"
    if ($Text -notmatch "vcs.revision=$ExpectedSHA") { throw "$Label binary revision mismatch" }
    if ($Text -notmatch "vcs.modified=false") { throw "$Label binary is marked modified" }
}

function Invoke-CatalogAndSuiteChecks([string]$Binary, [string]$LogDir) {
    [void](Invoke-Logged $Binary @("validate-catalog", "--catalog", "data/catalog.json") $RepoDir (Join-Path $LogDir "catalog-validation.txt"))
    foreach ($Version in @("v1", "v2")) {
        [void](Invoke-Logged $Binary @(
            "verify-search-suite",
            "--manifest", "benchmarks/suites/general-search-$Version.json",
            "--catalog", "data/catalog.json",
            "--lock", "benchmarks/suites/general-search-$Version.lock"
        ) $RepoDir (Join-Path $LogDir "suite-$Version-verification.txt"))
    }
}

Assert-ToolingFiles
Assert-DetachedCleanAt $RepoDir "measurement clone"
Assert-DetachedCleanAt $WebRepoDir "Web/WASM clone"

if ($Mode -eq "Preflight") {
    New-EmptyArtifactDir $ArtifactDir
    $LogDir = Join-Path $ArtifactDir "logs"
    $BuildDir = Join-Path $ArtifactDir "build"
    New-Item -ItemType Directory -Force -Path $LogDir, $BuildDir | Out-Null

    foreach ($Relative in @($ToolFiles.collector, $ToolFiles.extractor, $ToolFiles.freeze_script)) {
        Assert-PowerShellSyntax (Join-Path $ToolingRepoDir $Relative)
    }
    [void](Invoke-Logged "node" @("--check", (Join-Path $ToolingRepoDir $ToolFiles.analysis)) $ToolingRepoDir (Join-Path $LogDir "node-syntax.txt"))
    [void](Invoke-Logged "powershell.exe" @("-NoProfile", "-File", (Join-Path $ToolingRepoDir $ToolFiles.extractor), "-Preflight") $ToolingRepoDir (Join-Path $LogDir "extractor-preflight.txt"))
    [void](Invoke-Logged "powershell.exe" @("-NoProfile", "-File", (Join-Path $ToolingRepoDir $ToolFiles.freeze_script), "-Preflight") $ToolingRepoDir (Join-Path $LogDir "freeze-preflight.txt"))
    [void](Invoke-Logged "node" @((Join-Path $ToolingRepoDir $ToolFiles.analysis), "--preflight") $ToolingRepoDir (Join-Path $LogDir "analysis-preflight.txt"))

    foreach ($Relative in @(
        "data/catalog.json",
        "benchmarks/suites/general-search-v1.json",
        "benchmarks/suites/general-search-v1.lock",
        "benchmarks/suites/general-search-v2.json",
        "benchmarks/suites/general-search-v2.lock",
        "internal/benchmark/suite_generator.go",
        "internal/benchmark/suite_generator_v2.go"
    )) { Assert-File (Join-Path $RepoDir $Relative) }

    [void](Invoke-Logged "go" @("version") $RepoDir (Join-Path $LogDir "go-version.txt"))
    [void](Invoke-Logged "go" @("env", "GOOS", "GOARCH") $RepoDir (Join-Path $LogDir "go-env.txt"))
    [void](Invoke-Logged "go" @("tool", "pprof", "-help") $RepoDir (Join-Path $LogDir "pprof-help.txt"))
    Invoke-RepositoryGates $ArtifactDir -IncludeWeb

    $NormalBinary = Join-Path $BuildDir "r1ih-preflight-normal.exe"
    $ProfileBinary = Join-Path $BuildDir "r1ih-preflight-searchprofile.exe"
    [void](Invoke-Logged "go" @("build", "-buildvcs=true", "-o", $NormalBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $LogDir "build-normal.txt"))
    [void](Invoke-Logged "go" @("build", "-buildvcs=true", "-tags", "searchprofile", "-o", $ProfileBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $LogDir "build-searchprofile.txt"))
    Assert-BinaryMetadata $NormalBinary "preflight normal" (Join-Path $LogDir "normal-metadata.txt")
    Assert-BinaryMetadata $ProfileBinary "preflight searchprofile" (Join-Path $LogDir "searchprofile-metadata.txt")
    [void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--help") $RepoDir (Join-Path $LogDir "benchmark-help.txt") -AllowedExitCodes @(0, 2))
    Invoke-CatalogAndSuiteChecks $NormalBinary $LogDir

    $TestBinary = Join-Path $BuildDir "solver-preflight.test.exe"
    $TestProfile = Join-Path $BuildDir "solver-preflight.cpu.pprof"
    $OldSnapshotEnv = $env:R1I_SEMANTIC_SNAPSHOT
    try {
        $env:R1I_SEMANTIC_SNAPSHOT = Join-Path $BuildDir "pprof-semantic.json"
        [void](Invoke-Logged "go" @("test", "-run", "^TestR1ICrossBuildSemanticSnapshot$", "-count=1", "-cpuprofile", $TestProfile, "-o", $TestBinary, "./internal/solver") $RepoDir (Join-Path $LogDir "pprof-test-profile.txt"))
    } finally {
        $env:R1I_SEMANTIC_SNAPSHOT = $OldSnapshotEnv
    }
    [void](Invoke-Logged "go" @("tool", "pprof", "-top", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=0", $TestBinary, $TestProfile) $RepoDir (Join-Path $LogDir "pprof-top-smoke.txt"))
    [void](Invoke-Logged "go" @("tool", "pprof", "-tree", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=50", $TestBinary, $TestProfile) $RepoDir (Join-Path $LogDir "pprof-tree-smoke.txt"))

    $Hashes = Get-ToolHashes
    $Record = @(
        "R1I-H TOOLING PREFLIGHT",
        "status=PASS",
        "frozen_sha=$ExpectedSHA",
        "measurement_clone_clean_detached=true",
        "web_clone_clean_detached_at_start=true",
        "powershell_syntax=PASS",
        "node_syntax=PASS",
        "paths=PASS",
        "go_tools=PASS",
        "pprof_commands=PASS",
        "repository_tests=PASS",
        "semantic_snapshot=PASS",
        "catalog_and_suite_locks=PASS",
        "web_wasm_build=PASS",
        "binary_build_and_vcs=PASS",
        "benchmark_scenarios_runs=0",
        "tooling_protocol_sha256=$($Hashes.protocol)",
        "tooling_collector_sha256=$($Hashes.collector)",
        "tooling_extractor_sha256=$($Hashes.extractor)",
        "tooling_analysis_sha256=$($Hashes.analysis)",
        "tooling_freeze_script_sha256=$($Hashes.freeze_script)",
        "completed_at=$([DateTimeOffset]::Now.ToString('o'))"
    )
    Write-Utf8Text (Join-Path $ArtifactDir "preflight-record.txt") (($Record -join "`n") + "`n")
    Write-Output ($Record -join "`n")
    return
}

if ($ProtocolCommit -notmatch '^[0-9a-f]{40}$') { throw "Official mode requires a 40-character ProtocolCommit" }
if ([string]::IsNullOrWhiteSpace($PreflightRecord)) { throw "Official mode requires PreflightRecord" }
$PreflightRecord = [IO.Path]::GetFullPath($PreflightRecord)
$Preflight = Read-KeyValues $PreflightRecord
if ($Preflight.status -ne "PASS" -or $Preflight.benchmark_scenarios_runs -ne "0") { throw "preflight record is not a zero-benchmark PASS" }
if ($Preflight.frozen_sha -ne $ExpectedSHA) { throw "preflight frozen SHA mismatch" }
$CurrentToolHashes = Get-ToolHashes
foreach ($Key in $ToolFiles.Keys) {
    $RecordKey = "tooling_${Key}_sha256"
    if ($Preflight[$RecordKey] -ne $CurrentToolHashes[$Key]) { throw "tool changed after preflight: $Key" }
}
$ToolingHead = ((& git -C $ToolingRepoDir rev-parse HEAD) -join "").Trim()
if ($LASTEXITCODE -ne 0 -or $ToolingHead -ne $ProtocolCommit) { throw "tooling checkout HEAD must equal protocol commit" }
[string[]]$ToolingStatus = @(& git -C $ToolingRepoDir status --porcelain=v1)
if ($ToolingStatus.Count -ne 0) { throw "tooling checkout must be clean for official collection" }

New-EmptyArtifactDir $ArtifactDir
foreach ($Directory in @("binaries", "operations", "profiles", "profiles/per-case", "provenance", "scenarios", "smoke")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $ArtifactDir $Directory) | Out-Null
}
$ProvenanceDir = Join-Path $ArtifactDir "provenance"
$GateDir = Join-Path $ProvenanceDir "gates"
Write-Utf8Text (Join-Path $ProvenanceDir "preflight-record.txt") ([IO.File]::ReadAllText($PreflightRecord))

Invoke-RepositoryGates $ProvenanceDir -IncludeWeb

$NormalBinary = Join-Path $ArtifactDir "binaries/r1ih-normal.exe"
$ProfileBinary = Join-Path $ArtifactDir "binaries/r1ih-searchprofile.exe"
[void](Invoke-Logged "go" @("build", "-buildvcs=true", "-o", $NormalBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $ProvenanceDir "build-normal.txt"))
[void](Invoke-Logged "go" @("build", "-buildvcs=true", "-tags", "searchprofile", "-o", $ProfileBinary, "./cmd/backpack-brawl-solver") $RepoDir (Join-Path $ProvenanceDir "build-searchprofile.txt"))
Assert-BinaryMetadata $NormalBinary "official normal" (Join-Path $ProvenanceDir "r1ih-normal.metadata.txt")
Assert-BinaryMetadata $ProfileBinary "official searchprofile" (Join-Path $ProvenanceDir "r1ih-searchprofile.metadata.txt")
Invoke-CatalogAndSuiteChecks $NormalBinary $GateDir

[void](Invoke-Logged $NormalBinary @(
    "materialize-search-suite",
    "--manifest", "benchmarks/suites/general-search-v2.json",
    "--catalog", "data/catalog.json",
    "--lock", "benchmarks/suites/general-search-v2.lock",
    "--roles", "development",
    "--out", (Join-Path $ArtifactDir "scenarios")
) $RepoDir (Join-Path $ProvenanceDir "materialization-command.txt"))

$Materialized = @(Get-ChildItem -LiteralPath (Join-Path $ArtifactDir "scenarios") -File -Filter "*.json" | Sort-Object Name)
$ExpectedNames = @($ExpectedCases | ForEach-Object { "$_.json" })
if ($Materialized.Count -ne 14 -or (($Materialized.Name -join ',') -ne ($ExpectedNames -join ','))) {
    throw "development materialization is not exactly gsv2-013..026"
}
Write-Utf8Text (Join-Path $ProvenanceDir "materialization.txt") ((@(
    "roles=development",
    "case_count=14",
    "cases=$($ExpectedCases -join ',')",
    "validation_materialized=false",
    "public_holdout_materialized=false",
    "private_holdout_materialized=false"
) -join "`n") + "`n")

$StableFiles = [ordered]@{
    catalog = "data/catalog.json"
    suite_v1_manifest = "benchmarks/suites/general-search-v1.json"
    suite_v1_lock = "benchmarks/suites/general-search-v1.lock"
    suite_manifest = "benchmarks/suites/general-search-v2.json"
    suite_lock = "benchmarks/suites/general-search-v2.lock"
    generator_registry = "internal/benchmark/suite_generator.go"
    generator_v2 = "internal/benchmark/suite_generator_v2.go"
}
$Provenance = [Collections.Generic.List[string]]::new()
$Provenance.Add("R1I-H official collection provenance")
$Provenance.Add("frozen_sha=$ExpectedSHA")
$Provenance.Add("protocol_commit=$ProtocolCommit")
$Provenance.Add("git_status=clean")
$Provenance.Add("detached_head=true")
$Provenance.Add("preflight_record_sha256=$((Get-FileHash -LiteralPath $PreflightRecord -Algorithm SHA256).Hash.ToLowerInvariant())")
foreach ($Entry in $ToolFiles.GetEnumerator()) {
    $Provenance.Add("$($Entry.Key)_git_blob=$(Get-GitBlob $ToolingRepoDir $ProtocolCommit $Entry.Value)")
    $Provenance.Add("$($Entry.Key)_sha256=$($CurrentToolHashes[$Entry.Key])")
}
foreach ($Entry in $StableFiles.GetEnumerator()) {
    $SourcePath = Join-Path $RepoDir $Entry.Value
    $Provenance.Add("$($Entry.Key)_git_blob=$(Get-GitBlob $RepoDir $ExpectedSHA $Entry.Value)")
    $Provenance.Add("$($Entry.Key)_sha256=$((Get-FileHash -LiteralPath $SourcePath -Algorithm SHA256).Hash.ToLowerInvariant())")
}
$Provenance.Add("normal_binary_sha256=$((Get-FileHash -LiteralPath $NormalBinary -Algorithm SHA256).Hash.ToLowerInvariant())")
$Provenance.Add("searchprofile_binary_sha256=$((Get-FileHash -LiteralPath $ProfileBinary -Algorithm SHA256).Hash.ToLowerInvariant())")
$Provenance.Add("go_version=$((& go version) -join ' ')")
$Provenance.Add("goos_goarch=$((& go env GOOS GOARCH) -join ',')")
$OS = Get-CimInstance Win32_OperatingSystem
$CPU = Get-CimInstance Win32_Processor | Select-Object -First 1
$Computer = Get-CimInstance Win32_ComputerSystem
$TimeZone = Get-TimeZone
$Provenance.Add("os_caption=$($OS.Caption)")
$Provenance.Add("os_version=$($OS.Version)")
$Provenance.Add("os_build=$($OS.BuildNumber)")
$Provenance.Add("cpu=$($CPU.Name)")
$Provenance.Add("logical_processors=$($Computer.NumberOfLogicalProcessors)")
$Provenance.Add("visible_ram_bytes=$($Computer.TotalPhysicalMemory)")
$Provenance.Add("timezone=$($TimeZone.Id)")
$Provenance.Add("operation_matrix_gsv1=cases:gsv2-013..026 budgets:250000,1000000 repeat:1 workers:1 operation_profile:true diagnostic:false")
$Provenance.Add("operation_matrix_v4=cases:gsv2-013..026 budgets:1000000 repeat:1 workers:1 operation_profile:true diagnostic:false")
$Provenance.Add("cpu_slice=$($ProfileCases -join ',') budget:1000000 repeat:1 workers:1 operation_profile:false diagnostic:false")
$Provenance.Add("validation_materialized=false")
$Provenance.Add("public_holdout_materialized=false")
$Provenance.Add("private_holdout_materialized=false")
$MeasurementStatus = @(& git -C $RepoDir status --porcelain=v1)
if ($MeasurementStatus.Count -ne 0) { throw "measurement clone became dirty before official runs" }
Write-Utf8Text (Join-Path $ProvenanceDir "provenance.txt") (($Provenance -join "`n") + "`n")

$ScenarioDir = Join-Path $ArtifactDir "scenarios"
[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--out", (Join-Path $ArtifactDir "smoke/r1ih-smoke-normal.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-normal.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--out", (Join-Path $ArtifactDir "smoke/r1ih-smoke-tagged-off.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-tagged-off.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", "gsv2-013", "--budgets", "250000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--operation-profile", "--out", (Join-Path $ArtifactDir "smoke/r1ih-smoke-tagged-on.json")) $RepoDir (Join-Path $ProvenanceDir "smoke-tagged-on.txt"))

[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $AllScenarios, "--budgets", "250000,1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--operation-profile", "--out", (Join-Path $ArtifactDir "operations/r1ih-gsv1.json")) $RepoDir (Join-Path $ProvenanceDir "operations-gsv1.txt"))
[void](Invoke-Logged $ProfileBinary @("summarize-operation-profile", "--out", (Join-Path $ArtifactDir "operations/r1ih-gsv1-summary.json"), (Join-Path $ArtifactDir "operations/r1ih-gsv1.json")) $RepoDir (Join-Path $ProvenanceDir "operations-gsv1-summary.txt"))
[void](Invoke-Logged $ProfileBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $AllScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "v4", "--operation-profile", "--out", (Join-Path $ArtifactDir "operations/r1ih-v4.json")) $RepoDir (Join-Path $ProvenanceDir "operations-v4.txt"))
[void](Invoke-Logged $ProfileBinary @("summarize-operation-profile", "--out", (Join-Path $ArtifactDir "operations/r1ih-v4-summary.json"), (Join-Path $ArtifactDir "operations/r1ih-v4.json")) $RepoDir (Join-Path $ProvenanceDir "operations-v4-summary.txt"))

[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $ProfileScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--cpu-profile", (Join-Path $ArtifactDir "profiles/r1ih-gsv1.cpu.pprof"), "--heap-profile", (Join-Path $ArtifactDir "profiles/r1ih-gsv1.heap.pprof"), "--out", (Join-Path $ArtifactDir "profiles/r1ih-gsv1-profile.json")) $RepoDir (Join-Path $ProvenanceDir "profile-gsv1-combined.txt"))
foreach ($Scenario in $ProfileCases) {
    [void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $Scenario, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "general-search-v1", "--cpu-profile", (Join-Path $ArtifactDir "profiles/per-case/$Scenario.cpu.pprof"), "--out", (Join-Path $ArtifactDir "profiles/per-case/$Scenario.json")) $RepoDir (Join-Path $ProvenanceDir "profile-$Scenario.txt"))
}
[void](Invoke-Logged $NormalBinary @("benchmark-scenarios", "--dir", $ScenarioDir, "--scenarios", $ProfileScenarios, "--budgets", "1000000", "--repeat", "1", "--workers", "1", "--constellation-seed-variant", "v4", "--cpu-profile", (Join-Path $ArtifactDir "profiles/r1ih-v4.cpu.pprof"), "--out", (Join-Path $ArtifactDir "profiles/r1ih-v4-profile.json")) $RepoDir (Join-Path $ProvenanceDir "profile-v4-combined.txt"))

Write-Utf8Text (Join-Path $ProvenanceDir "official-collection-complete.txt") ((@(
    "status=PASS",
    "operation_profile_runs=42",
    "combined_gsv1_cpu=true",
    "per_case_gsv1_cpu=6",
    "combined_v4_cpu=true",
    "gsv1_heap=true",
    "last_solver_run=combined_v4_cpu",
    "freeze_required_next=true",
    "completed_at=$([DateTimeOffset]::Now.ToString('o'))"
) -join "`n") + "`n")
Write-Output "R1I-H official solver collection complete. Run r1ih-freeze.ps1 before any extractor, analyzer, or solver command."
