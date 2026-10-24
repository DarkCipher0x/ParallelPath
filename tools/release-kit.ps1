# Prepares dist\<version>\ for publishing. It publishes nothing: no GitHub, no website, no Discord.
#
#   1. checks every download against SHA256SUMS.txt (as built by build-release.ps1)
#   2. builds the source zip  ParallelPath-src-v<version>.zip  from an allowlist, behind a privacy gate
#      (names, local paths, server addresses; every image must be in tools\approved-images.txt, added after
#      someone has looked at it)
#   3. writes RELEASE-NOTES.md (from CHANGELOG.md), release-manifest.json (every file: role, label, site path,
#      bytes, SHA-256) and a SHA256SUMS.txt that covers everything published
#
# Run from anywhere:  powershell -File tools\release-kit.ps1 [-Channel beta] [-Repo owner/name -Commit SHA]
# With -Repo and -Commit (the public mirror's commit at tag v<version>), the manifest carries the GitHub release, the
# mirror and each file's GitHub asset URL; the source zip is then built from the mirror at that commit (git archive).
# Maintainer notes (RESUME*, PLAN*, OUTSTANDING*) and machine files never go into the source zip.
param([string]$Channel = 'beta', [string]$Repo = '', [string]$Commit = '', [string]$Mirror = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$version = ([IO.File]::ReadAllText("$root\VERSION")).Trim()
$out = Join-Path $root "dist\$version"
Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
function Step($text) { Write-Host "`n== $text" -ForegroundColor Cyan }
function Sha256($file) { (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLower() }
if (-not (Test-Path $out)) { throw "dist\$version doesn't exist: run tools\build-release.ps1 first" }

# ---- 1. The downloads match what was built
Step "Downloads"
$artefacts = [ordered]@{
    "ParallelPath-Setup-$version.exe"                 = @('app-windows', 'Windows app (setup, no admin rights needed)')
    "ParallelPath-$version-android.apk"               = @('app-android', 'Android app (.apk, signed)')
    "ParallelPath-$version-android-emulator.apk"      = @('app-android-emulator', 'Android app for x86-64 emulators (.apk)')
    "ParallelPath-Server-Setup-$version.exe"          = @('server-windows', 'Server for Windows (setup wizard)')
    "ParallelPath-Server-$version-linux-x64.tar.gz"   = @('server-linux', 'Server for Ubuntu 24.04+ (installer, .tar.gz)')
    "android-signing-certificate.txt"                 = @('signing', 'Android signing certificate fingerprint')
}
$sums = @{}
foreach ($line in [IO.File]::ReadAllLines("$out\SHA256SUMS.txt")) {
    if ($line -match '^([0-9a-f]{64})\s+\*?(.+)$') { $sums[$Matches[2].Trim()] = $Matches[1] }
}
foreach ($name in $artefacts.Keys) {
    $f = Join-Path $out $name
    if (-not (Test-Path $f)) { throw "missing: $name" }
    if ($name -ne 'android-signing-certificate.txt') {
        if (-not $sums.ContainsKey($name)) { throw "$name isn't in SHA256SUMS.txt" }
        if ((Sha256 $f) -ne $sums[$name]) { throw "$name doesn't match SHA256SUMS.txt" }
    }
}
Write-Host "ok ($($artefacts.Count) files)"

# ---- 2. Source zip from an allowlist, behind the privacy gate
Step "Source"
$include = 'core', 'server', 'engine', 'clients', 'installer', 'docs', 'tools', 'LICENSE', 'VERSION', 'README.md', 'CHANGELOG.md', 'go.work', 'go.work.sum'
$skipDir = '^(build|\.gradle|\.kotlin|\.idea|\.vs|bin|obj|dist|node_modules|Payload|out|captures)$'
$skipFile = '(?i)(\.(exe|dll|apk|aab|aar|so|pdb|p12|pfx|jks|keystore|zip|gz|tgz|class|log|tmp|user|suo)$)|(^local\.properties$)|(^(RESUME|PLAN|OUTSTANDING)[^/\\]*$)|(\.sha256$)'
$files = New-Object System.Collections.Generic.List[IO.FileInfo]
function Walk([IO.DirectoryInfo]$d) {
    foreach ($c in $d.GetDirectories()) { if ($c.Name -notmatch $skipDir) { Walk $c } }
    foreach ($f in $d.GetFiles()) { if ($f.Name -notmatch $skipFile) { $files.Add($f) } }
}
foreach ($i in $include) {
    $p = Join-Path $root $i
    if (Test-Path $p -PathType Container) { Walk (Get-Item $p) } elseif (Test-Path $p) { $files.Add((Get-Item $p)) } else { throw "allowlist entry missing: $i" }
}
# Privacy gate: no account names, local paths, private servers or personal details in anything that ships.
# The personal terms (names, places, server addresses: one regex per line) live outside the repo, so this script
# doesn't publish them itself.
$termsFile = Join-Path $env:LOCALAPPDATA 'ParallelPath-release\privacy-terms.txt'
if (-not (Test-Path $termsFile)) { throw "no privacy terms at $termsFile (one regex per line)" }
$u = [regex]::Escape($env:USERNAME)
$terms = @("\\Users\\$u", "/Users/$u", "(?<![a-z])$u(?![a-z])", '@gmail\.com') +
    @([IO.File]::ReadAllLines($termsFile) | Where-Object { $_.Trim() -and -not $_.StartsWith('#') })
$gate = $terms | ForEach-Object { New-Object regex $_, 'IgnoreCase' }
$approved = @{}
if (Test-Path "$PSScriptRoot\approved-images.txt") {
    foreach ($l in [IO.File]::ReadAllLines("$PSScriptRoot\approved-images.txt")) { if ($l -match '^([0-9a-f]{64})') { $approved[$Matches[1]] = $true } }
}
$problems = @()
foreach ($f in $files) {
    $rel = $f.FullName.Substring($root.Length + 1)
    if ($f.Extension -match '(?i)^\.(png|jpe?g|gif|webp|ico|bmp|svg)$' -and -not $approved.ContainsKey((Sha256 $f.FullName))) {
        $problems += "image not approved (look at it, then add its SHA-256 to tools\approved-images.txt): $rel"
    }
    if ($f.Name -eq 'model.bin') { continue } # the tone model's weights: random bytes, checked by its own tests
    $text = [Text.Encoding]::GetEncoding(28591).GetString([IO.File]::ReadAllBytes($f.FullName))
    foreach ($g in $gate) { if ($g.IsMatch($text)) { $problems += "'$g' in $rel" } }
}
if ($problems) { $problems | ForEach-Object { Write-Host $_ -ForegroundColor Red }; throw "privacy gate: $($problems.Count) problem(s)" }
$zipName = "ParallelPath-src-v$version.zip"
$zip = Join-Path $out $zipName
Remove-Item $zip -ErrorAction SilentlyContinue
if ($Mirror -and $Commit) {
    # The release's source is exactly the public mirror at its tagged commit.
    & git -C $Mirror archive --format=zip "--prefix=ParallelPath-$version/" -o $zip $Commit
    if ($LASTEXITCODE) { throw 'git archive failed' }
} else {
    $fs = [IO.File]::Open($zip, 'CreateNew')
    $archive = New-Object IO.Compression.ZipArchive($fs, [IO.Compression.ZipArchiveMode]::Create)
    foreach ($f in ($files | Sort-Object FullName)) {
        $name = "ParallelPath-$version/" + $f.FullName.Substring($root.Length + 1).Replace('\', '/')
        [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $f.FullName, $name, [IO.Compression.CompressionLevel]::Optimal) | Out-Null
    }
    $archive.Dispose(); $fs.Dispose()
}
Write-Host "$zipName : $($files.Count) files, $([math]::Round((Get-Item $zip).Length / 1MB, 1)) MB, privacy gate clean"

# ---- 3. Release notes, manifest, checksums
Step "Notes and manifest"
$log = [IO.File]::ReadAllText("$root\CHANGELOG.md")
$m = [regex]::Match($log, "(?ms)^## $([regex]::Escape($version))\b.*?(?=^## |\z)")
if (-not $m.Success) { throw "CHANGELOG.md has no entry for $version" }
$beta = $Channel -eq 'beta'
$notes = "# ParallelPath $version$(if ($beta) { ' (beta)' })`n`n" + ($m.Value -replace "(?m)^## [^\n]*\n", '').Trim() + "`n`n" +
    "Downloads are checked by SHA-256 (SHA256SUMS.txt). Guides, the security test report and the full source are " +
    "included. $(if ($beta) { 'Beta software' } else { 'Free software' }), provided as is (DISCLAIMER). The Windows setups aren't code-signed yet.`n"
[IO.File]::WriteAllText("$out\RELEASE-NOTES.md", $notes, (New-Object Text.UTF8Encoding $false))

$entries = @()
function Add([string]$file, [string]$role, [string]$label, [string]$site) {
    $p = Join-Path $out $file
    $e = [ordered]@{ file = $file; role = $role; label = $label; site_path = $site; bytes = (Get-Item $p).Length; sha256 = (Sha256 $p) }
    if ($Repo) { $e.github_asset = "https://github.com/$Repo/releases/download/v$version/$(Split-Path $file -Leaf)" }
    $script:entries += $e
}
foreach ($name in $artefacts.Keys) { if ($artefacts[$name][0] -ne 'signing') { Add $name $artefacts[$name][0] $artefacts[$name][1] "downloads/$name" } }
Add $zipName 'source' 'Source code (.zip)' "downloads/source/$zipName"
Add 'RELEASE-NOTES.md' 'notes' 'Release notes' "downloads/RELEASE-NOTES-$version.md"
Add 'TESTING.md' 'notes' 'What to test in this build' "downloads/TESTING-$version.md"
foreach ($g in Get-ChildItem "$out\guides" -Filter *.pdf | Sort-Object Name) {
    Add "guides/$($g.Name)" 'guide' ($g.BaseName -replace '-', ' ') "downloads/guides/$($g.Name)"
}
$manifest = [ordered]@{
    app = 'parallelpath'; version = $version; released = (Get-Date).ToString('yyyy-MM-dd'); channel = $Channel; beta = $beta
    published = $false
    github_release = $(if ($Repo) { "https://github.com/$Repo/releases/tag/v$version" } else { $null })
    mirror = $(if ($Repo) { [ordered]@{ repo = $Repo; tag = "v$version"; commit = $Commit } } else { $null })
    android_signing = ([IO.File]::ReadAllText("$out\android-signing-certificate.txt")).Trim()
    rule = 'Publish exactly these files under these paths, byte for byte. Nothing is published until the owner says go.'
    files = $entries
}
[IO.File]::WriteAllText("$out\release-manifest.json", ($manifest | ConvertTo-Json -Depth 5), (New-Object Text.UTF8Encoding $false))
$lines = $entries | ForEach-Object { "$($_.sha256)  $($_.file)" }
[IO.File]::WriteAllText("$out\SHA256SUMS.txt", (($lines -join "`n") + "`n"), (New-Object Text.UTF8Encoding $false))
Write-Host "release-manifest.json: $($entries.Count) files; SHA256SUMS.txt rewritten to cover them"
Write-Host "`nReady in dist\$version (not published)." -ForegroundColor Green
