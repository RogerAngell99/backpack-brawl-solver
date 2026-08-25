[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactDir,

    [string[]]$ExtraSymbol = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ArtifactDir = [IO.Path]::GetFullPath($ArtifactDir)
$Binary = Join-Path $ArtifactDir "binaries\solver.exe"
$DerivedDir = Join-Path $ArtifactDir "derived"
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ProfileCases = @("gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024")

$DefaultSymbols = @(
    [ordered]@{ id = "plateau-select"; regexp = "selectPlateauEntries" },
    [ordered]@{ id = "plateau-observe"; regexp = "plateauArchive.*observe" },
    [ordered]@{ id = "outgoing-upper-priority"; regexp = "outgoingBoundContext.*upperPriorityCounts" },
    [ordered]@{ id = "outgoing-source-hit"; regexp = "sourceHitsTargetWithCatalog" },
    [ordered]@{ id = "coverage-placement-key"; regexp = "coveragePlacementKey" },
    [ordered]@{ id = "filtered-removed-options"; regexp = "filteredRemovedOptions" },
    [ordered]@{ id = "canonical-copy-order"; regexp = "placementRespectsCanonicalCopyOrder" },
    [ordered]@{ id = "physical-instance-ids"; regexp = "physicalInstanceIDs" },
    [ordered]@{ id = "placement-key"; regexp = "placementKey" },
    [ordered]@{ id = "priority-upper-bound"; regexp = "partialRepairV3PriorityUpperBound" },
    [ordered]@{ id = "placement-index"; regexp = "placementByInstanceID" }
)

function Assert-File([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "required file missing: $Path"
    }
}

function Write-Utf8Lines([string]$Path, [string[]]$Lines) {
    $Parent = Split-Path -Parent $Path
    if ($Parent) {
        New-Item -ItemType Directory -Force -Path $Parent | Out-Null
    }
    [IO.File]::WriteAllLines($Path, $Lines, $Utf8NoBom)
}

function Write-Utf8Text([string]$Path, [string]$Value) {
    $Parent = Split-Path -Parent $Path
    if ($Parent) {
        New-Item -ItemType Directory -Force -Path $Parent | Out-Null
    }
    [IO.File]::WriteAllText($Path, $Value, $Utf8NoBom)
}

function Get-RelativeArtifactPath([string]$Path) {
    $Root = $ArtifactDir.TrimEnd('\') + '\'
    if (-not $Path.StartsWith($Root, [StringComparison]::OrdinalIgnoreCase)) {
        throw "path is outside artifact directory: $Path"
    }
    return $Path.Substring($Root.Length).Replace('\', '/')
}

function Invoke-Pprof([string[]]$Arguments, [string]$Profile, [string]$OutputPath, [switch]$AllowNoMatches) {
    Assert-File $Binary
    Assert-File $Profile
    [string[]]$Output = & go tool pprof @Arguments $Binary $Profile 2>&1 | ForEach-Object { [string]$_ }
    $ExitCode = $LASTEXITCODE
    Write-Utf8Lines $OutputPath $Output
    $NoMatches = (($Output -join "`n") -match 'no matches found')
    if ($ExitCode -ne 0 -and -not ($AllowNoMatches -and $NoMatches)) {
        throw "pprof failed ($ExitCode): go tool pprof $($Arguments -join ' '); see $OutputPath"
    }
    return ,$Output
}

function Convert-Seconds([string]$Token) {
    if ($Token -eq "." -or $Token -eq "0") { return 0.0 }
    if ($Token -notmatch '^(?<value>[0-9]+(?:\.[0-9]+)?)s$') {
        throw "unexpected CPU quantity: $Token"
    }
    return [double]::Parse($Matches.value, [Globalization.CultureInfo]::InvariantCulture)
}

function Convert-HeapValue([string]$Token, [string]$Unit) {
    if ($Token -eq "0") { return 0.0 }
    $Suffix = if ($Unit -eq "bytes") { "B" } else { "objects" }
    if (-not $Token.EndsWith($Suffix, [StringComparison]::Ordinal)) {
        throw "unexpected $Unit quantity: $Token"
    }
    $Number = $Token.Substring(0, $Token.Length - $Suffix.Length)
    return [double]::Parse($Number, [Globalization.CultureInfo]::InvariantCulture)
}

function Convert-Fraction([string]$Token) {
    if ($Token -notmatch '^(?<value>[0-9]+(?:\.[0-9]+)?)%$') {
        throw "unexpected percentage: $Token"
    }
    return [double]::Parse($Matches.value, [Globalization.CultureInfo]::InvariantCulture) / 100.0
}

function Parse-Top([string[]]$Lines, [string]$Unit) {
    $Total = $null
    $Rows = @()
    foreach ($Line in $Lines) {
        if ($Line -match ' of (?<total>\S+) total$') {
            $Total = if ($Unit -eq "seconds") { Convert-Seconds $Matches.total } else { Convert-HeapValue $Matches.total $Unit }
        }
        if ($Line -match '^\s*(?<flat>\S+)\s+(?<flatpct>[0-9.]+%)\s+(?<sumpct>[0-9.]+%)\s+(?<cum>\S+)\s+(?<cumpct>[0-9.]+%)\s+(?<symbol>.+?)\s*$') {
            $Rows += [ordered]@{
                rank = $Rows.Count + 1
                symbol = $Matches.symbol
                flat = if ($Unit -eq "seconds") { Convert-Seconds $Matches.flat } else { Convert-HeapValue $Matches.flat $Unit }
                flat_fraction = Convert-Fraction $Matches.flatpct
                cum = if ($Unit -eq "seconds") { Convert-Seconds $Matches.cum } else { Convert-HeapValue $Matches.cum $Unit }
                cum_fraction = Convert-Fraction $Matches.cumpct
            }
        }
    }
    if ($null -eq $Total -or $Total -le 0) { throw "pprof total was not parsed" }
    if ($Rows.Count -eq 0) { throw "pprof table contained no rows" }
    return [ordered]@{ total = $Total; rows = $Rows }
}

function Parse-Source([string[]]$Lines, [string]$Id, [string]$Regexp, [string]$TextPath) {
    $Routines = @()
    $Current = $null
    foreach ($Line in $Lines) {
        if ($Line -match '^ROUTINE =+ (?<symbol>.+) in (?<file>.+)$') {
            if ($null -ne $Current) { $Routines += $Current }
            $Current = [ordered]@{ symbol = $Matches.symbol; file = $Matches.file; lines = @() }
            continue
        }
        if ($null -ne $Current -and $Line -match '^\s*(?<flat>\.|[0-9]+(?:\.[0-9]+)?s)\s+(?<cum>\.|[0-9]+(?:\.[0-9]+)?s)\s+(?<line>[0-9]+):(?<source>.*)$') {
            $Current.lines += [ordered]@{
                line = [int]$Matches.line
                flat_seconds = Convert-Seconds $Matches.flat
                cum_seconds = Convert-Seconds $Matches.cum
                source = $Matches.source
            }
        }
    }
    if ($null -ne $Current) { $Routines += $Current }
    return [ordered]@{
        id = $Id
        regexp = $Regexp
        text_path = Get-RelativeArtifactPath $TextPath
        routines = $Routines
    }
}

function Get-ExtraSymbolSpecs([string[]]$Values) {
    $Specs = @()
    $Index = 0
    foreach ($Value in $Values) {
        if ([string]::IsNullOrWhiteSpace($Value)) { continue }
        $Index++
        $Slug = ($Value.ToLowerInvariant() -replace '[^a-z0-9]+', '-').Trim('-')
        if ($Slug.Length -gt 42) { $Slug = $Slug.Substring(0, 42).Trim('-') }
        if (-not $Slug) { $Slug = "symbol" }
        $Specs += [ordered]@{ id = "extra-$('{0:D2}' -f $Index)-$Slug"; regexp = $Value }
    }
    return $Specs
}

function Extract-CpuProfile([string]$Id, [string]$Profile, [string]$OutputPrefix, [object[]]$Symbols) {
    $TopPath = "$OutputPrefix-top.txt"
    $CumPath = "$OutputPrefix-top-cum.txt"
    $TreePath = "$OutputPrefix-tree.txt"
    $TopLines = Invoke-Pprof @("-top", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") $Profile $TopPath
    $CumLines = Invoke-Pprof @("-top", "-cum", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") $Profile $CumPath
    [void](Invoke-Pprof @("-tree", "-unit=seconds", "-nodefraction=0", "-edgefraction=0", "-nodecount=500") $Profile $TreePath)
    $Top = Parse-Top $TopLines "seconds"
    $Cum = Parse-Top $CumLines "seconds"
    if ([Math]::Abs($Top.total - $Cum.total) -gt 0.000001) { throw "top/cum total mismatch for $Id" }

    $SourceExtracts = [ordered]@{}
    foreach ($Symbol in $Symbols) {
        $SourcePath = "$OutputPrefix-source-$($Symbol.id).txt"
        $SourceLines = Invoke-Pprof -Arguments @("-list=$($Symbol.regexp)", "-unit=seconds") -Profile $Profile -OutputPath $SourcePath -AllowNoMatches
        $SourceExtracts[$Symbol.id] = Parse-Source $SourceLines $Symbol.id $Symbol.regexp $SourcePath
    }

    return [ordered]@{
        id = $Id
        raw_profile = Get-RelativeArtifactPath $Profile
        total_seconds = $Top.total
        top = $Top.rows
        top_cum = $Cum.rows
        source_extracts = $SourceExtracts
    }
}

Assert-File $Binary
New-Item -ItemType Directory -Force -Path $DerivedDir | Out-Null
$ExtraSpecs = Get-ExtraSymbolSpecs $ExtraSymbol
$Symbols = @($DefaultSymbols) + @($ExtraSpecs)

$CombinedCpu = Join-Path $ArtifactDir "profiles\r1if-gsv1.cpu.pprof"
$CombinedHeap = Join-Path $ArtifactDir "profiles\r1if-gsv1.heap.pprof"
$V4Cpu = Join-Path $ArtifactDir "profiles\r1if-v4.cpu.pprof"

$Combined = Extract-CpuProfile "combined_gsv1" $CombinedCpu (Join-Path $DerivedDir "cpu") $Symbols
[void](Invoke-Pprof @("-peek=backpack-brawl-solver/internal/", "-unit=seconds", "-nodefraction=0", "-edgefraction=0") $CombinedCpu (Join-Path $DerivedDir "cpu-callers.txt"))
$V4 = Extract-CpuProfile "combined_v4" $V4Cpu (Join-Path $DerivedDir "v4\cpu") $Symbols

$PerCase = [ordered]@{}
foreach ($Scenario in $ProfileCases) {
    $Profile = Join-Path $ArtifactDir "profiles\per-case\$Scenario.cpu.pprof"
    $Prefix = Join-Path $DerivedDir "per-case\$Scenario"
    $PerCase[$Scenario] = Extract-CpuProfile $Scenario $Profile $Prefix $Symbols
}

$HeapProfiles = [ordered]@{}
foreach ($HeapSpec in @(
    [ordered]@{ sample_index = "alloc_space"; unit = "bytes"; output = "heap-alloc-space.txt" },
    [ordered]@{ sample_index = "alloc_objects"; unit = "objects"; output = "heap-alloc-objects.txt" },
    [ordered]@{ sample_index = "inuse_space"; unit = "bytes"; output = "heap-inuse-space.txt" }
)) {
    $OutputPath = Join-Path $DerivedDir $HeapSpec.output
    $Lines = Invoke-Pprof @("-sample_index=$($HeapSpec.sample_index)", "-top", "-unit=$($HeapSpec.unit)", "-nodefraction=0", "-edgefraction=0", "-nodecount=0") $CombinedHeap $OutputPath
    $Parsed = Parse-Top $Lines $HeapSpec.unit
    $HeapProfiles[$HeapSpec.sample_index] = [ordered]@{
        sample_index = $HeapSpec.sample_index
        raw_profile = Get-RelativeArtifactPath $CombinedHeap
        total = $Parsed.total
        unit = $HeapSpec.unit
        top = $Parsed.rows
    }
}

$Canonical = [ordered]@{
    version = "r1if-canonical-profile-v1"
    generated_by = "r1if-profile-extract.ps1"
    binary = Get-RelativeArtifactPath $Binary
    extract_invocation = [ordered]@{
        default_symbols = $DefaultSymbols
        extra_symbols = @($ExtraSymbol)
    }
    cpu_profiles = [ordered]@{
        combined_gsv1 = $Combined
        combined_v4 = $V4
        per_case = $PerCase
    }
    heap_profiles = $HeapProfiles
}

$CanonicalPath = Join-Path $DerivedDir "canonical-profile-data.json"
Write-Utf8Text $CanonicalPath (($Canonical | ConvertTo-Json -Depth 100) + "`n")
Write-Output "Wrote canonical R1I-F profile data to $CanonicalPath"
