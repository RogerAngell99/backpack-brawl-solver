[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("Preflight", "Official")]
    [string]$Mode,

    [Parameter(Mandatory = $true)]
    [string]$MeasurementRepo,

    [Parameter(Mandatory = $true)]
    [string]$ToolingRepo,

    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [string]$ToolingCommit,

    [string]$PreflightRecord
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ExpectedSHA = "11644a7d88bd4e4bdd1f97977f8aad5e59391293"
$MeasurementRepo = [IO.Path]::GetFullPath($MeasurementRepo)
$ToolingRepo = [IO.Path]::GetFullPath($ToolingRepo)
$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir)
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ExpectedCases = @(13..26 | ForEach-Object { "gsv2-$('{0:D3}' -f $_)" })
$AllScenarios = $ExpectedCases -join ","
$ToolFiles = [ordered]@{
    protocol = "benchmarks/efficacy/e1a-protocol.md"
    collector = "benchmarks/efficacy/e1a-collect.ps1"
    analyzer = "benchmarks/efficacy/e1a-analysis.mjs"
    analyzer_test = "benchmarks/efficacy/e1a-analysis.test.mjs"
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

function Assert-DetachedCleanAt([string]$Path, [string]$Label, [string]$Expected) {
    $Head = ((& git -C $Path rev-parse HEAD) -join "").Trim()
    if ($LASTEXITCODE -ne 0 -or $Head -ne $Expected) { throw "$Label HEAD=$Head, expected $Expected" }
    [string[]]$Status = @(& git -C $Path status --porcelain=v1)
    if ($LASTEXITCODE -ne 0 -or $Status.Count -ne 0) { throw "$Label must be clean" }
    $Branch = ((& git -C $Path branch --show-current) -join "").Trim()
    if ($Branch) { throw "$Label must be detached, found branch $Branch" }
}

function Assert-PowerShellSyntax([string]$Path) {
    $Tokens = $null
    $Errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile($Path, [ref]$Tokens, [ref]$Errors)
    if (@($Errors).Count -ne 0) { throw "PowerShell syntax failure: $((@($Errors) | ForEach-Object Message) -join '; ')" }
}

function Get-ToolHashes {
    $Hashes = [ordered]@{}
    foreach ($Entry in $ToolFiles.GetEnumerator()) {
        $Path = Join-Path $ToolingRepo $Entry.Value
        Assert-File $Path
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

function Assert-BinaryMetadata([string]$Binary, [string]$LogPath) {
    [string[]]$Metadata = @(Invoke-Logged "go" @("version", "-m", $Binary) $MeasurementRepo $LogPath)
    $Text = $Metadata -join "`n"
    if ($Text -notmatch "vcs.revision=$ExpectedSHA") { throw "binary revision mismatch" }
    if ($Text -notmatch "vcs.modified=false") { throw "binary is marked modified" }
}

function Invoke-CatalogAndSuiteChecks([string]$Binary, [string]$LogDir) {
    [void](Invoke-Logged $Binary @("validate-catalog", "--catalog", "data/catalog.json") $MeasurementRepo (Join-Path $LogDir "catalog-validation.txt"))
    [void](Invoke-Logged $Binary @(
        "verify-search-suite",
        "--manifest", "benchmarks/suites/general-search-v2.json",
        "--catalog", "data/catalog.json",
        "--lock", "benchmarks/suites/general-search-v2.lock"
    ) $MeasurementRepo (Join-Path $LogDir "suite-v2-verification.txt"))
}

function Get-SystemRecord {
    $Cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
    $OS = Get-CimInstance Win32_OperatingSystem | Select-Object -First 1
    return @(
        "go_version=$((& go version) -join '')",
        "goos_goarch=$(((& go env GOOS GOARCH) -join ',').Trim())",
        "os_caption=$($OS.Caption)",
        "os_version=$($OS.Version)",
        "os_build=$($OS.BuildNumber)",
        "cpu=$($Cpu.Name)",
        "logical_processors=$($Cpu.NumberOfLogicalProcessors)",
        "visible_ram_bytes=$([int64]$OS.TotalVisibleMemorySize * 1024)",
        "timezone=$([TimeZoneInfo]::Local.Id)"
    )
}

function Assert-Report([string]$Path, [string]$Variant, [int[]]$Budgets, [bool]$Diagnostic) {
    Assert-File $Path
    $Report = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($Report.build_revision -ne $ExpectedSHA) { throw "report revision mismatch: $Path" }
    if ($Report.constellation_seed_variant -ne $Variant) { throw "report variant mismatch: $Path" }
    if ([bool]$Report.diagnostic -ne $Diagnostic) { throw "report diagnostic mismatch: $Path" }
    if ($null -ne $Report.PSObject.Properties["operation_profiling"] -and [bool]$Report.operation_profiling) { throw "operation profiling unexpectedly enabled: $Path" }
    if (($Report.budgets -join ',') -ne ($Budgets -join ',')) { throw "report budget mismatch: $Path" }
    $ExpectedRuns = $ExpectedCases.Count * $Budgets.Count
    if (@($Report.runs).Count -ne $ExpectedRuns) { throw "report run count mismatch: $Path" }
    $ActualKeys = @($Report.runs | ForEach-Object { "$($_.scenario)|$($_.budget)|$($_.repeat)" } | Sort-Object)
    $ExpectedKeys = @($ExpectedCases | ForEach-Object { $Scenario = $_; $Budgets | ForEach-Object { "$Scenario|$_|1" } } | Sort-Object)
    if (($ActualKeys -join ',') -ne ($ExpectedKeys -join ',')) { throw "report run keys mismatch: $Path" }
    foreach ($Run in $Report.runs) {
        if ($Run.error) { throw "solver error in $Path for $($Run.scenario): $($Run.error)" }
    }
}

function Invoke-Matrix([string]$Binary, [string]$ScenarioDir, [string]$Variant, [string]$Budgets, [bool]$Diagnostic, [string]$OutPath, [string]$LogPath) {
    [string[]]$Arguments = @(
        "benchmark-scenarios",
        "--dir", $ScenarioDir,
        "--scenarios", $AllScenarios,
        "--budgets", $Budgets,
        "--repeat", "1",
        "--workers", "1",
        "--constellation-seed-variant", $Variant,
        "--out", $OutPath
    )
    if ($Diagnostic) { $Arguments = $Arguments[0..($Arguments.Count - 3)] + @("--diagnostic") + $Arguments[($Arguments.Count - 2)..($Arguments.Count - 1)] }
    [void](Invoke-Logged $Binary $Arguments $MeasurementRepo $LogPath)
}

function Get-RelativeSlashPath([string]$Root, [string]$File) {
    return [IO.Path]::GetRelativePath($Root, $File).Replace('\', '/')
}

function Freeze-Artifact([string]$Root) {
    $Files = @(Get-ChildItem -LiteralPath $Root -Recurse -File | Sort-Object FullName)
    $Entries = @()
    $HashLines = @()
    [int64]$TotalBytes = 0
    foreach ($File in $Files) {
        $Relative = Get-RelativeSlashPath $Root $File.FullName
        $Hash = (Get-FileHash -LiteralPath $File.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $TotalBytes += $File.Length
        $Entries += [ordered]@{ path = $Relative; bytes = [int64]$File.Length; sha256 = $Hash }
        $HashLines += "$Hash  $Relative"
    }
    Write-Utf8Text (Join-Path $Root "RAW-SHA256SUMS.txt") (($HashLines -join "`n") + "`n")
    $Manifest = [ordered]@{
        schema_version = 1
        baseline_sha = $ExpectedSHA
        raw_file_count = $Entries.Count
        raw_bytes = $TotalBytes
        files = $Entries
    }
    $ManifestPath = Join-Path $Root "raw-manifest.json"
    Write-Utf8Text $ManifestPath (($Manifest | ConvertTo-Json -Depth 8) + "`n")
    $ManifestHash = (Get-FileHash -LiteralPath $ManifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-Utf8Text (Join-Path $Root "manifest-sha256.txt") "$ManifestHash  raw-manifest.json`n"
    Write-Utf8Text (Join-Path $Root "freeze-record.txt") ((@(
        "status=PASS",
        "baseline_sha=$ExpectedSHA",
        "raw_file_count=$($Entries.Count)",
        "raw_bytes=$TotalBytes",
        "raw_manifest_sha256=$ManifestHash",
        "raw_files_read_only=true",
        "post_freeze_solver_runs=0",
        "frozen_at=$([DateTimeOffset]::Now.ToString('o'))"
    ) -join "`n") + "`n")
    foreach ($File in @(Get-ChildItem -LiteralPath $Root -Recurse -File)) { $File.IsReadOnly = $true }
    return [ordered]@{ Count = $Entries.Count; Bytes = $TotalBytes; ManifestHash = $ManifestHash }
}

foreach ($Relative in $ToolFiles.Values) { Assert-File (Join-Path $ToolingRepo $Relative) }
Assert-DetachedCleanAt $MeasurementRepo "measurement clone" $ExpectedSHA
Assert-PowerShellSyntax (Join-Path $ToolingRepo $ToolFiles.collector)

if ($Mode -eq "Preflight") {
    New-EmptyArtifactDir $ArtifactDir
    $LogDir = Join-Path $ArtifactDir "logs"
    $BuildDir = Join-Path $ArtifactDir "build"
    New-Item -ItemType Directory -Force -Path $LogDir, $BuildDir | Out-Null
    [void](Invoke-Logged "node" @("--check", (Join-Path $ToolingRepo $ToolFiles.analyzer)) $ToolingRepo (Join-Path $LogDir "node-check.txt"))
    [void](Invoke-Logged "node" @("--test", (Join-Path $ToolingRepo $ToolFiles.analyzer_test)) $ToolingRepo (Join-Path $LogDir "node-test.txt"))
    [void](Invoke-Logged "node" @((Join-Path $ToolingRepo $ToolFiles.analyzer), "--preflight") $ToolingRepo (Join-Path $LogDir "analyzer-preflight.txt"))
    [void](Invoke-Logged "go" @("test", "./internal/benchmark/...", "./internal/solver/...") $MeasurementRepo (Join-Path $LogDir "go-test.txt"))
    $Binary = Join-Path $BuildDir "e1a-preflight.exe"
    [void](Invoke-Logged "go" @("build", "-buildvcs=true", "-o", $Binary, "./cmd/backpack-brawl-solver") $MeasurementRepo (Join-Path $LogDir "build.txt"))
    Assert-BinaryMetadata $Binary (Join-Path $LogDir "binary-metadata.txt")
    Invoke-CatalogAndSuiteChecks $Binary $LogDir
    $Hashes = Get-ToolHashes
    $Record = @(
        "E1-A TOOLING PREFLIGHT",
        "status=PASS",
        "baseline_sha=$ExpectedSHA",
        "measurement_clone_clean_detached=true",
        "powershell_syntax=PASS",
        "node_syntax=PASS",
        "analyzer_tests=PASS",
        "repository_tests=PASS",
        "catalog_and_suite_lock=PASS",
        "binary_build_and_vcs=PASS",
        "benchmark_scenarios_runs=0"
    )
    foreach ($Key in $ToolFiles.Keys) { $Record += "tooling_${Key}_sha256=$($Hashes[$Key])" }
    $Record += "completed_at=$([DateTimeOffset]::Now.ToString('o'))"
    Write-Utf8Text (Join-Path $ArtifactDir "preflight-record.txt") (($Record -join "`n") + "`n")
    Write-Output ($Record -join "`n")
    return
}

if ($ToolingCommit -notmatch '^[0-9a-f]{40}$') { throw "Official mode requires a 40-character ToolingCommit" }
Assert-DetachedCleanAt $ToolingRepo "tooling clone" $ToolingCommit
if ([string]::IsNullOrWhiteSpace($PreflightRecord)) { throw "Official mode requires PreflightRecord" }
$PreflightRecord = [IO.Path]::GetFullPath($PreflightRecord)
$Preflight = Read-KeyValues $PreflightRecord
if ($Preflight.status -ne "PASS" -or $Preflight.benchmark_scenarios_runs -ne "0") { throw "preflight record is not a zero-benchmark PASS" }
if ($Preflight.baseline_sha -ne $ExpectedSHA) { throw "preflight baseline SHA mismatch" }
$ToolHashes = Get-ToolHashes
foreach ($Key in $ToolFiles.Keys) {
    if ($Preflight["tooling_${Key}_sha256"] -ne $ToolHashes[$Key]) { throw "tool changed after preflight: $Key" }
}

New-EmptyArtifactDir $ArtifactDir
foreach ($Directory in @("binaries", "logs", "provenance", "raw/quality", "raw/diagnostic", "scenarios")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $ArtifactDir $Directory) | Out-Null
}
$LogDir = Join-Path $ArtifactDir "logs"
$ProvenanceDir = Join-Path $ArtifactDir "provenance"
Write-Utf8Text (Join-Path $ProvenanceDir "preflight-record.txt") ([IO.File]::ReadAllText($PreflightRecord))

$Binary = Join-Path $ArtifactDir "binaries/e1a-solver.exe"
[void](Invoke-Logged "go" @("build", "-buildvcs=true", "-o", $Binary, "./cmd/backpack-brawl-solver") $MeasurementRepo (Join-Path $LogDir "build.txt"))
Assert-BinaryMetadata $Binary (Join-Path $LogDir "binary-metadata.txt")
Invoke-CatalogAndSuiteChecks $Binary $LogDir

$ScenarioDir = Join-Path $ArtifactDir "scenarios"
[void](Invoke-Logged $Binary @(
    "materialize-search-suite",
    "--manifest", "benchmarks/suites/general-search-v2.json",
    "--catalog", "data/catalog.json",
    "--lock", "benchmarks/suites/general-search-v2.lock",
    "--roles", "development",
    "--out", $ScenarioDir
) $MeasurementRepo (Join-Path $LogDir "materialization.txt"))
$Materialized = @(Get-ChildItem -LiteralPath $ScenarioDir -File -Filter "*.json" | Sort-Object Name)
$ExpectedNames = @($ExpectedCases | ForEach-Object { "$_.json" })
if ($Materialized.Count -ne 14 -or (($Materialized.Name -join ',') -ne ($ExpectedNames -join ','))) { throw "development materialization is not exactly gsv2-013..026" }

$StableFiles = [ordered]@{
    catalog = "data/catalog.json"
    suite_manifest = "benchmarks/suites/general-search-v2.json"
    suite_lock = "benchmarks/suites/general-search-v2.lock"
    suite_generator_registry = "internal/benchmark/suite_generator.go"
    suite_generator_v2 = "internal/benchmark/suite_generator_v2.go"
}
$Provenance = @(
    "E1-A official collection provenance",
    "baseline_sha=$ExpectedSHA",
    "tooling_commit=$ToolingCommit",
    "measurement_git_status=clean",
    "measurement_detached_head=true",
    "tooling_git_status=clean",
    "tooling_detached_head=true",
    "preflight_record_sha256=$((Get-FileHash -LiteralPath $PreflightRecord -Algorithm SHA256).Hash.ToLowerInvariant())"
)
foreach ($Entry in $ToolFiles.GetEnumerator()) {
    $Provenance += "$($Entry.Key)_git_blob=$(Get-GitBlob $ToolingRepo $ToolingCommit $Entry.Value)"
    $Provenance += "$($Entry.Key)_sha256=$($ToolHashes[$Entry.Key])"
}
foreach ($Entry in $StableFiles.GetEnumerator()) {
    $Source = Join-Path $MeasurementRepo $Entry.Value
    $Provenance += "$($Entry.Key)_git_blob=$(Get-GitBlob $MeasurementRepo $ExpectedSHA $Entry.Value)"
    $Provenance += "$($Entry.Key)_sha256=$((Get-FileHash -LiteralPath $Source -Algorithm SHA256).Hash.ToLowerInvariant())"
}
$Provenance += "binary_sha256=$((Get-FileHash -LiteralPath $Binary -Algorithm SHA256).Hash.ToLowerInvariant())"
$Provenance += Get-SystemRecord
$Provenance += @(
    "quality_matrix=general-search-v1:250000,1000000;v4:1000000;v5:1000000;v5.1:1000000;cases:gsv2-013..026;repeat:1;workers:1;diagnostic:false;operation_profile:false",
    "diagnostic_twin_matrix=general-search-v1:250000,1000000;v4:1000000;v5:1000000;v5.1:1000000;cases:gsv2-013..026;repeat:1;workers:1;diagnostic:true;operation_profile:false",
    "quality_runs=70",
    "diagnostic_runs=70",
    "roles_materialized=development",
    "cases=$($ExpectedCases -join ',')",
    "validation_materialized=false",
    "public_holdout_materialized=false",
    "private_holdout_materialized=false"
)
Write-Utf8Text (Join-Path $ProvenanceDir "provenance.txt") (($Provenance -join "`n") + "`n")

$Matrices = @(
    @{ Variant = "general-search-v1"; Budgets = "250000,1000000" },
    @{ Variant = "v4"; Budgets = "1000000" },
    @{ Variant = "v5"; Budgets = "1000000" },
    @{ Variant = "v5.1"; Budgets = "1000000" }
)
foreach ($Matrix in $Matrices) {
    $Variant = [string]$Matrix.Variant
    $Budgets = [string]$Matrix.Budgets
    $BudgetValues = @($Budgets.Split(',') | ForEach-Object { [int]$_ })
    $QualityPath = Join-Path $ArtifactDir "raw/quality/$Variant.json"
    $DiagnosticPath = Join-Path $ArtifactDir "raw/diagnostic/$Variant.json"
    Invoke-Matrix $Binary $ScenarioDir $Variant $Budgets $false $QualityPath (Join-Path $LogDir "quality-$Variant.txt")
    Assert-Report $QualityPath $Variant $BudgetValues $false
    Invoke-Matrix $Binary $ScenarioDir $Variant $Budgets $true $DiagnosticPath (Join-Path $LogDir "diagnostic-$Variant.txt")
    Assert-Report $DiagnosticPath $Variant $BudgetValues $true
}

Write-Utf8Text (Join-Path $ProvenanceDir "collection-complete.txt") ((@(
    "status=PASS",
    "quality_runs=70",
    "diagnostic_runs=70",
    "last_solver_run=diagnostic-v5.1",
    "freeze_immediate=true",
    "completed_at=$([DateTimeOffset]::Now.ToString('o'))"
) -join "`n") + "`n")
$Freeze = Freeze-Artifact $ArtifactDir
Write-Output "E1-A official collection frozen: files=$($Freeze.Count) bytes=$($Freeze.Bytes) manifest=$($Freeze.ManifestHash)"
