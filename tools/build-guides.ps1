# Prints the guides into dist\<version>\guides\ as Markdown and PDF (Edge, which every Windows PC has, prints them
# headless with a throwaway profile). build-release.ps1 runs it; run it on its own after changing only the docs.
#
#   powershell -File tools\build-guides.ps1 [-Out dist\<version>]
param([string]$Out = '')
. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not $Out) { $Out = Join-Path $root ("dist\" + ([IO.File]::ReadAllText("$root\VERSION")).Trim()) }
$guides = Join-Path $Out 'guides'
$work = Join-Path $env:TEMP ('pp-guides-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Force $guides, $work | Out-Null
try {
    Get-ChildItem $guides -File | Remove-Item -Force # no stale guide (an older security report) is left behind
    $docs = @('USER-GUIDE', 'ADMIN-GUIDE', 'FAQ', 'SECURITY', 'TONE-MODEL', 'FEATURES', 'PRIVACY', 'DISCLAIMER') +
        @(Get-ChildItem "$root\docs" -Filter 'PENTEST-*.md' | Sort-Object LastWriteTime | Select-Object -Last 1 | ForEach-Object BaseName)
    $md = $docs | ForEach-Object { "$root\docs\$_.md" }
    Push-Location $root
    try { & go run ./tools/guides "$work\html" @md | Out-Null; if ($LASTEXITCODE) { throw 'guides failed' } } finally { Pop-Location }
    $edge = @("${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe", "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe") |
        Where-Object { Test-Path $_ } | Select-Object -First 1
    foreach ($d in $docs) {
        Copy-Item "$root\docs\$d.md" "$guides\$d.md"
        $html = "file:///" + "$work\html\$d.html".Replace('\', '/')
        Start-Process $edge -ArgumentList '--headless=new', '--disable-gpu', '--no-first-run', '--disable-extensions', "--user-data-dir=$work\edge-profile",
            '--no-pdf-header-footer', "--print-to-pdf=$guides\$d.pdf", $html -Wait -WindowStyle Hidden
        if (-not (Test-Path "$guides\$d.pdf")) { throw "couldn't print $d.pdf" }
    }
    Write-Host "guides: $($docs.Count) as .md and .pdf in $guides"
} finally {
    # Edge's helper processes can outlive the print a moment; they hold the throwaway profile open.
    Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" | Where-Object { $_.CommandLine -like "*$work*" } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 1
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
