[CmdletBinding()]
param(
    [string]$ArtifactDir,
    [switch]$Preflight
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ProfileCases = @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")
$ProjectPrefix = "backpack-brawl-solver/internal/"
$SolverRegexp = "backpack-brawl-solver/internal/solver"

if ($Preflight) {
    Write-Output "status=PASS"
    Write-Output "candidate_specific_symbols=false"
    Write-Output "project_source_regexp=$SolverRegexp"
    Write-Output "required_outputs=cpu-top.tsv,cpu-top-cum.tsv,cpu-project-owned.tsv,cpu-hot-source-lines.tsv,heap-alloc-space.tsv,heap-alloc-objects.tsv,case-attribution.tsv"
    return
}
if ([string]::IsNullOrWhiteSpace($ArtifactDir)) { throw "ArtifactDir is required outside preflight" }

$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir).TrimEnd('\')
$Binary = Join-Path $ArtifactDir "binaries/r1ih-normal.exe"
$DerivedDir = Join-Path $ArtifactDir "derived"

function Assert-File([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "required file missing: $Path" }
}

function Write-Utf8Text([string]$Path, [string]$Value) {
    $Parent = Split-Path -Parent $Path
    if ($Parent) { New-Item -ItemType Directory -Force -Path $Parent | Out-Null }
    [IO.File]::WriteAllText($Path, $Value, $Utf8NoBom)
}

function Get-RelativeArtifactPath([string]$Path) {
    $Root = $ArtifactDir + '\'
    $Full = [IO.Path]::GetFullPath($Path)
    if (-not $Full.StartsWith($Root, [StringComparison]::OrdinalIgnoreCase)) { throw "path escaped artifact root: $Path" }
    return $Full.Substring($Root.Length).Replace('\', '/')
}

function Invoke-Pprof([string[]]$Arguments, [string]$Profile, [string]$OutputPath) {
    Assert-File $Binary
    Assert-File $Profile
    [string[]]$Output = @(& go tool pprof @Arguments $Binary $Profile 2>&1 | ForEach-Object { [string]$_ })
    $ExitCode = $LASTEXITCODE
    Write-Utf8Text $OutputPath (($Output -join "`n") + "`n")
    if ($ExitCode -ne 0) { throw "pprof failed ($ExitCode): go tool pprof $($Arguments -join ' '); see $OutputPath" }
    return ,$Output
}

function Convert-TimeSeconds([string]$Token) {
    if ($Token -eq "." -or $Token -eq "0") { return 0.0 }
    if ($Token -notmatch '^(?<value>[0-9]+(?:\.[0-9]+)?)(?<unit>ns|us|µs|ms|s|mins?|hrs?)$') { throw "unexpected CPU quantity: $Token" }
    $Value = [double]::Parse($Matches.value, [Globalization.CultureInfo]::InvariantCulture)
    switch -Regex ($Matches.unit) {
        '^ns$' { return $Value / 1e9 }
        '^(us|µs)$' { return $Value / 1e6 }
        '^ms$' { return $Value / 1e3 }
        '^s$' { return $Value }
        '^mins?$' { return $Value * 60 }
        '^hrs?$' { return $Value * 3600 }
    }
}

function Convert-HeapValue([string]$Token, [string]$Unit) {
    if ($Token -eq "." -or $Token -eq "0") { return 0.0 }
    if ($Unit -eq "objects") {
        $Number = $Token -replace 'objects$', ''
        if ($Number -notmatch '^[0-9]+(?:\.[0-9]+)?$') { throw "unexpected object quantity: $Token" }
        return [double]::Parse($Number, [Globalization.CultureInfo]::InvariantCulture)
    }
    if ($Token -notmatch '^(?<value>[0-9]+(?:\.[0-9]+)?)(?<unit>B|kB|MB|GB|TB)$') { throw "unexpected byte quantity: $Token" }
    $Value = [double]::Parse($Matches.value, [Globalization.CultureInfo]::InvariantCulture)
    switch ($Matches.unit) {
        'B' { return $Value }
        'kB' { return $Value * 1e3 }
        'MB' { return $Value * 1e6 }
        'GB' { return $Value * 1e9 }
        'TB' { return $Value * 1e12 }
    }
}

function Convert-Fraction([string]$Token) {
    if ($Token -notmatch '^(?<value>[0-9]+(?:\.[0-9]+)?)%$') { throw "unexpected percentage: $Token" }
    return [double]::Parse($Matches.value, [Globalization.CultureInfo]::InvariantCulture) / 100.0
}

function Parse-Top([string[]]$Lines, [string]$Unit) {
    $Total = $null
    $Rows = @()
    foreach ($Line in $Lines) {
        if ($Line -match ' of (?<total>\S+) total$') {
            $Total = if ($Unit -eq "seconds") { Convert-TimeSeconds $Matches.total } else { Convert-HeapValue $Matches.total $Unit }
        }
        if ($Line -match '^\s*(?<flat>\S+)\s+(?<flatpct>[0-9.]+%)\s+(?<sumpct>[0-9.]+%)\s+(?<cum>\S+)\s+(?<cumpct>[0-9.]+%)\s+(?<symbol>.+?)\s*$') {
            $Rows += [ordered]@{
                rank = $Rows.Count + 1
                symbol = $Matches.symbol -replace ' \(inline\)$', ''
                flat = if ($Unit -eq "seconds") { Convert-TimeSeconds $Matches.flat } else { Convert-HeapValue $Matches.flat $Unit }
                flat_fraction = Convert-Fraction $Matches.flatpct
                cumulative = if ($Unit -eq "seconds") { Convert-TimeSeconds $Matches.cum } else { Convert-HeapValue $Matches.cum $Unit }
                cumulative_fraction = Convert-Fraction $Matches.cumpct
            }
        }
    }
    if ($null -eq $Total -or $Total -le 0) { throw "pprof total was not parsed" }
    if ($Rows.Count -eq 0) { throw "pprof table contained no rows" }
    return [ordered]@{ total = $Total; rows = $Rows }
}

function Parse-Source([string[]]$Lines) {
    $Routines = @()
    $Current = $null
    foreach ($Line in $Lines) {
        if ($Line -match '^ROUTINE =+ (?<symbol>.+) in (?<file>.+)$') {
            if ($null -ne $Current) { $Routines += $Current }
            $Current = [ordered]@{
                symbol = ($Matches.symbol -replace ' \(inline\)$', '')
                file = Split-Path -Leaf $Matches.file
                lines = @()
            }
            continue
        }
        if ($null -ne $Current -and $Line -match '^\s*(?<flat>\.|\S+)\s+(?<cum>\.|\S+)\s+(?<line>[0-9]+):(?<source>.*)$') {
            try {
                $Flat = Convert-TimeSeconds $Matches.flat
                $Cum = Convert-TimeSeconds $Matches.cum
            } catch {
                continue
            }
            $Current.lines += [ordered]@{
                line = [int]$Matches.line
                flat_seconds = $Flat
                cumulative_seconds = $Cum
                source = $Matches.source
            }
        }
    }
    if ($null -ne $Current) { $Routines += $Current }
    if ($Routines.Count -eq 0) { throw "project source listing contained no routines" }
    return $Routines
}

function Extract-Cpu([string]$Id, [string]$Profile, [string]$Prefix, [switch]$WithTree) {
    $TopText = "$Prefix-top.txt"
    $CumText = "$Prefix-top-cum.txt"
    $SourceText = "$Prefix-source-all-project.txt"
    [string[]]$TopLines = @(Invoke-Pprof @("-top", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") $Profile $TopText)
    [string[]]$CumLines = @(Invoke-Pprof @("-top", "-cum", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") $Profile $CumText)
    [string[]]$SourceLines = @(Invoke-Pprof @("-list=$SolverRegexp", "-unit=seconds") $Profile $SourceText)
    if ($WithTree) {
        [void](Invoke-Pprof @("-peek=$ProjectPrefix", "-unit=seconds", "-nodefraction=0", "-edgefraction=0") $Profile "$Prefix-callers.txt")
        [void](Invoke-Pprof @("-tree", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=500") $Profile "$Prefix-callee-tree.txt")
    }
    $Top = Parse-Top $TopLines "seconds"
    $Cum = Parse-Top $CumLines "seconds"
    if ([Math]::Abs($Top.total - $Cum.total) -gt 0.000001) { throw "top/cumulative total mismatch for $Id" }
    return [ordered]@{
        id = $Id
        raw_profile = Get-RelativeArtifactPath $Profile
        total_seconds = $Top.total
        top = $Top.rows
        top_cumulative = $Cum.rows
        source_routines = Parse-Source $SourceLines
    }
}

function Escape-Tsv([object]$Value) {
    if ($null -eq $Value) { return "" }
    return ([string]$Value).Replace("`t", " ").Replace("`r", " ").Replace("`n", " ")
}

function Write-Tsv([string]$Path, [string[]]$Headers, [object[]]$Rows) {
    $Lines = [Collections.Generic.List[string]]::new()
    $Lines.Add(($Headers -join "`t"))
    foreach ($Row in $Rows) {
        $Lines.Add((($Headers | ForEach-Object { Escape-Tsv $Row[$_] }) -join "`t"))
    }
    Write-Utf8Text $Path (($Lines -join "`n") + "`n")
}

function Project-Owned([object[]]$Rows) {
    return @($Rows | Where-Object { $_.symbol.StartsWith($ProjectPrefix, [StringComparison]::Ordinal) })
}

function Find-FunctionRow([object]$Profile, [string]$Symbol, [string]$Table) {
    $Rows = if ($Table -eq "flat") { $Profile.top } else { $Profile.top_cumulative }
    $Matches = @($Rows | Where-Object { $_.symbol -eq $Symbol })
    if ($Matches.Count -gt 1) { throw "ambiguous function row $($Profile.id)/$Symbol" }
    if ($Matches.Count -eq 0) { return $null }
    return $Matches[0]
}

function Find-SourceLine([object]$Profile, [string]$Routine, [string]$File, [int]$Line) {
    $Routines = @($Profile.source_routines | Where-Object { $_.symbol -eq $Routine -and $_.file -eq $File })
    if ($Routines.Count -gt 1) { throw "ambiguous source routine $($Profile.id)/$Routine/$File" }
    if ($Routines.Count -eq 0) { return $null }
    $Lines = @($Routines[0].lines | Where-Object { $_.line -eq $Line })
    if ($Lines.Count -gt 1) { throw "ambiguous source line $($Profile.id)/$Routine/$File`:$Line" }
    if ($Lines.Count -eq 0) { return $null }
    return $Lines[0]
}

Assert-File $Binary
New-Item -ItemType Directory -Force -Path $DerivedDir, (Join-Path $DerivedDir "per-case"), (Join-Path $DerivedDir "v4") | Out-Null

$Combined = Extract-Cpu "combined_gsv1" (Join-Path $ArtifactDir "profiles/r1ih-gsv1.cpu.pprof") (Join-Path $DerivedDir "cpu") -WithTree
$V4 = Extract-Cpu "combined_v4" (Join-Path $ArtifactDir "profiles/r1ih-v4.cpu.pprof") (Join-Path $DerivedDir "v4/cpu") -WithTree
$PerCase = [ordered]@{}
foreach ($Scenario in $ProfileCases) {
    $PerCase[$Scenario] = Extract-Cpu $Scenario (Join-Path $ArtifactDir "profiles/per-case/$Scenario.cpu.pprof") (Join-Path $DerivedDir "per-case/$Scenario")
}

$HeapProfiles = [ordered]@{}
foreach ($Spec in @(
    [ordered]@{ index = "alloc_space"; unit = "bytes"; text = "heap-alloc-space.txt" },
    [ordered]@{ index = "alloc_objects"; unit = "objects"; text = "heap-alloc-objects.txt" },
    [ordered]@{ index = "inuse_space"; unit = "bytes"; text = "heap-inuse-space.txt" }
)) {
    $TextPath = Join-Path $DerivedDir $Spec.text
    [string[]]$Lines = @(Invoke-Pprof @("-sample_index=$($Spec.index)", "-top", "-unit=$($Spec.unit)", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") (Join-Path $ArtifactDir "profiles/r1ih-gsv1.heap.pprof") $TextPath)
    $Parsed = Parse-Top $Lines $Spec.unit
    $HeapProfiles[$Spec.index] = [ordered]@{
        sample_index = $Spec.index
        raw_profile = "profiles/r1ih-gsv1.heap.pprof"
        total = $Parsed.total
        unit = $Spec.unit
        top = $Parsed.rows
    }
}

$ProjectFlat = @(Project-Owned $Combined.top)
$ProjectCum = @(Project-Owned $Combined.top_cumulative)
$ProjectSymbols = @($ProjectFlat.symbol + $ProjectCum.symbol | Sort-Object -Unique)
$ProjectRows = foreach ($Symbol in $ProjectSymbols) {
    $Flat = Find-FunctionRow $Combined $Symbol "flat"
    $Cum = Find-FunctionRow $Combined $Symbol "cum"
    [ordered]@{
        symbol = $Symbol
        flat_rank = if ($null -ne $Flat) { $Flat.rank } else { "" }
        flat_seconds = if ($null -ne $Flat) { $Flat.flat } else { 0 }
        flat_fraction = if ($null -ne $Flat) { $Flat.flat_fraction } else { 0 }
        cumulative_rank = if ($null -ne $Cum) { $Cum.rank } else { "" }
        cumulative_seconds = if ($null -ne $Cum) { $Cum.cumulative } else { 0 }
        cumulative_fraction = if ($null -ne $Cum) { $Cum.cumulative_fraction } else { 0 }
    }
}

$HotLines = @()
foreach ($Routine in $Combined.source_routines) {
    foreach ($Line in $Routine.lines) {
        $FlatFraction = $Line.flat_seconds / $Combined.total_seconds
        $CumFraction = $Line.cumulative_seconds / $Combined.total_seconds
        if ($FlatFraction -ge 0.01 -or $CumFraction -ge 0.01) {
            $HotLines += [ordered]@{
                routine = $Routine.symbol
                file = $Routine.file
                line = $Line.line
                flat_seconds = $Line.flat_seconds
                flat_fraction = $FlatFraction
                cumulative_seconds = $Line.cumulative_seconds
                cumulative_fraction = $CumFraction
                source = $Line.source.Trim()
            }
        }
    }
}
$HotLines = @($HotLines | Sort-Object @{ Expression = "cumulative_seconds"; Descending = $true }, @{ Expression = "flat_seconds"; Descending = $true })

$Inventory = [Collections.Generic.List[object]]::new()
foreach ($Row in $ProjectRows) {
    $Triggers = [Collections.Generic.List[string]]::new()
    if ([double]$Row.flat_fraction -ge 0.01) { $Triggers.Add("project_flat_ge_1pct") }
    if ([double]$Row.cumulative_fraction -ge 0.015) { $Triggers.Add("project_cumulative_ge_1_5pct") }
    $FlatProjectRank = [Array]::IndexOf(@($ProjectFlat.symbol), $Row.symbol) + 1
    $CumProjectRank = [Array]::IndexOf(@($ProjectCum.symbol), $Row.symbol) + 1
    if ($FlatProjectRank -ge 1 -and $FlatProjectRank -le 20) { $Triggers.Add("project_flat_top20") }
    if ($CumProjectRank -ge 1 -and $CumProjectRank -le 20) { $Triggers.Add("project_cumulative_top20") }
    if ($Triggers.Count -gt 0) {
        $Inventory.Add([ordered]@{
            key = "function:$($Row.symbol)"
            kind = "function"
            symbol = $Row.symbol
            triggers = @($Triggers)
            flat_seconds = $Row.flat_seconds
            flat_fraction = $Row.flat_fraction
            cumulative_seconds = $Row.cumulative_seconds
            cumulative_fraction = $Row.cumulative_fraction
            project_flat_rank = if ($FlatProjectRank -ge 1) { $FlatProjectRank } else { $null }
            project_cumulative_rank = if ($CumProjectRank -ge 1) { $CumProjectRank } else { $null }
        })
    }
}
foreach ($Row in $HotLines) {
    $Inventory.Add([ordered]@{
        key = "source:$($Row.routine):$($Row.file):$($Row.line)"
        kind = "source_line"
        routine = $Row.routine
        file = $Row.file
        line = $Row.line
        triggers = @("hot_source_line_ge_1pct")
        flat_seconds = $Row.flat_seconds
        flat_fraction = $Row.flat_fraction
        cumulative_seconds = $Row.cumulative_seconds
        cumulative_fraction = $Row.cumulative_fraction
        source = $Row.source
    })
}
$AllocSpaceProject = @(Project-Owned $HeapProfiles.alloc_space.top)
foreach ($Row in $AllocSpaceProject | Where-Object { $_.flat_fraction -ge 0.01 }) {
    $Inventory.Add([ordered]@{
        key = "heap:alloc_space:$($Row.symbol)"
        kind = "heap_alloc_space"
        symbol = $Row.symbol
        triggers = @("project_alloc_space_ge_1pct")
        value = $Row.flat
        fraction = $Row.flat_fraction
    })
}
$AllocObjectsProject = @(Project-Owned $HeapProfiles.alloc_objects.top)
for ($Index = 0; $Index -lt [Math]::Min(10, $AllocObjectsProject.Count); $Index++) {
    $Row = $AllocObjectsProject[$Index]
    $Inventory.Add([ordered]@{
        key = "heap:alloc_objects:$($Row.symbol)"
        kind = "heap_alloc_objects"
        symbol = $Row.symbol
        triggers = @("project_alloc_objects_top10")
        value = $Row.flat
        fraction = $Row.flat_fraction
        project_rank = $Index + 1
    })
}
$DuplicateKeys = @($Inventory | Group-Object key | Where-Object Count -ne 1)
if ($DuplicateKeys.Count -ne 0) { throw "duplicate mechanical inventory keys" }

$TopHeaders = @("rank", "symbol", "flat", "flat_fraction", "cumulative", "cumulative_fraction")
Write-Tsv (Join-Path $DerivedDir "cpu-top.tsv") $TopHeaders $Combined.top
Write-Tsv (Join-Path $DerivedDir "cpu-top-cum.tsv") $TopHeaders $Combined.top_cumulative
Write-Tsv (Join-Path $DerivedDir "cpu-project-owned.tsv") @("symbol", "flat_rank", "flat_seconds", "flat_fraction", "cumulative_rank", "cumulative_seconds", "cumulative_fraction") $ProjectRows
Write-Tsv (Join-Path $DerivedDir "cpu-hot-source-lines.tsv") @("routine", "file", "line", "flat_seconds", "flat_fraction", "cumulative_seconds", "cumulative_fraction", "source") $HotLines
Write-Tsv (Join-Path $DerivedDir "heap-alloc-space.tsv") $TopHeaders $HeapProfiles.alloc_space.top
Write-Tsv (Join-Path $DerivedDir "heap-alloc-objects.tsv") $TopHeaders $HeapProfiles.alloc_objects.top

$CaseRows = @()
foreach ($Entry in $Inventory | Where-Object { $_.kind -in @("function", "source_line") }) {
    foreach ($Scenario in $ProfileCases) {
        $Profile = $PerCase[$Scenario]
        if ($Entry.kind -eq "function") {
            $Flat = Find-FunctionRow $Profile $Entry.symbol "flat"
            $Cum = Find-FunctionRow $Profile $Entry.symbol "cum"
            $FlatSeconds = if ($null -ne $Flat) { $Flat.flat } else { 0 }
            $CumSeconds = if ($null -ne $Cum) { $Cum.cumulative } else { 0 }
        } else {
            $Line = Find-SourceLine $Profile $Entry.routine $Entry.file $Entry.line
            $FlatSeconds = if ($null -ne $Line) { $Line.flat_seconds } else { 0 }
            $CumSeconds = if ($null -ne $Line) { $Line.cumulative_seconds } else { 0 }
        }
        $CaseRows += [ordered]@{
            inventory_key = $Entry.key
            scenario = $Scenario
            total_seconds = $Profile.total_seconds
            flat_seconds = $FlatSeconds
            flat_fraction = $FlatSeconds / $Profile.total_seconds
            cumulative_seconds = $CumSeconds
            cumulative_fraction = $CumSeconds / $Profile.total_seconds
        }
    }
}
Write-Tsv (Join-Path $DerivedDir "case-attribution.tsv") @("inventory_key", "scenario", "total_seconds", "flat_seconds", "flat_fraction", "cumulative_seconds", "cumulative_fraction") $CaseRows

$Canonical = [ordered]@{
    version = "r1ih-canonical-profile-v1"
    generated_by = "r1ih-profile-extract.ps1"
    binary = "binaries/r1ih-normal.exe"
    project_prefix = $ProjectPrefix
    candidate_specific_symbols = $false
    cpu_profiles = [ordered]@{
        combined_gsv1 = $Combined
        combined_v4 = $V4
        per_case = $PerCase
    }
    heap_profiles = $HeapProfiles
    eligible_inventory = @($Inventory | Sort-Object key)
}
Write-Utf8Text (Join-Path $DerivedDir "canonical-profile-data.json") (($Canonical | ConvertTo-Json -Depth 100) + "`n")
Write-Output "status=PASS"
Write-Output "canonical_profile=$(Join-Path $DerivedDir 'canonical-profile-data.json')"
Write-Output "eligible_inventory_entries=$($Inventory.Count)"
Write-Output "candidate_specific_symbols=false"
