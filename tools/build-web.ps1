# Builds the ParallelPath web app and puts it into the server (server\internal\webapp\dist, embedded in ppserver):
#
#   the UI          clients\apps\web  (Compose, Kotlin/Wasm)  -> parallelpath.js, *.wasm, index.html, host.js...
#   the engine      engine\cmd\ppweb  (Go, WebAssembly)       -> ppweb.wasm
#   the Go loader   $GOROOT\lib\wasm\wasm_exec.js
#
# Run from anywhere:  powershell -File tools\build-web.ps1   (build-release.ps1 runs it before building ppserver)
param([string]$Version = '')

. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not $Version) { $Version = ([IO.File]::ReadAllText("$root\VERSION")).Trim() }
function Run($what) { & $what; if ($LASTEXITCODE -ne 0) { throw "failed: $what" } }

$dist = "$root\server\internal\webapp\dist"
$ui = "$root\clients\apps\web\build\dist\wasmJs\productionExecutable"

Write-Host "== Web app UI (Kotlin/Wasm)" -ForegroundColor Cyan
Push-Location "$root\clients"
try { Run { .\gradlew.bat :apps:web:wasmJsBrowserDistribution --console=plain -q } } finally { Pop-Location }

Write-Host "== Web app engine (Go/WebAssembly)" -ForegroundColor Cyan
$tmp = Join-Path $env:TEMP ("ppweb-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $tmp | Out-Null
Push-Location "$root\engine"
try {
    $env:GOOS = 'js'; $env:GOARCH = 'wasm'
    Run { go build -ldflags "-s -w -buildid= -X main.version=$Version" -o "$tmp\ppweb.wasm" ./cmd/ppweb }
} finally { Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue; Pop-Location }

Write-Host "== Into the server" -ForegroundColor Cyan
# Empty the folder rather than deleting it (an open shell or editor inside it would make deleting it fail).
New-Item -ItemType Directory -Force $dist | Out-Null
Get-ChildItem $dist -Force | Remove-Item -Recurse -Force
# A placeholder straight away, so ppserver still compiles (go:embed needs a file) if anything below fails.
[IO.File]::WriteAllText("$dist\index.html", "<!doctype html><html lang=`"en`"><head><meta charset=`"utf-8`"><title>ParallelPath</title></head>" +
    "<body><p>The ParallelPath web app isn't included in this build of the server (tools/build-release.ps1 adds it).</p></body></html>`n")
Get-ChildItem $ui -Recurse -File | Where-Object { $_.Extension -ne '.map' -and $_.Name -notlike '*.LICENSE.txt' } | ForEach-Object {
    $rel = $_.FullName.Substring($ui.Length + 1)
    $to = Join-Path $dist $rel
    New-Item -ItemType Directory -Force (Split-Path $to) | Out-Null
    Copy-Item $_.FullName $to
}
Copy-Item "$tmp\ppweb.wasm" "$dist\ppweb.wasm"
Copy-Item (Join-Path $env:GOROOT 'lib\wasm\wasm_exec.js') "$dist\wasm_exec.js"
New-Item -ItemType Directory "$dist\call" | Out-Null
Copy-Item "$root\engine\callui\web\*" "$dist\call\"   # the call page (the same files the Windows and Android engines serve)
Remove-Item -Recurse -Force $tmp
# webpack writes import.meta.url of bundled modules as the build machine's absolute file:// path (only a fallback for
# locating skiko's wasm, which the bundle loads by its hashed name): make it relative, so no local path ships.
$js = "$dist\parallelpath.js"
$text = [IO.File]::ReadAllText($js)
$text = [regex]::Replace($text, '"file:///[^"]*?/(kotlin/[^"/]+)"', '"./$1"')
[IO.File]::WriteAllText($js, $text, (New-Object Text.UTF8Encoding $false))
foreach ($f in Get-ChildItem $dist -Recurse -File) {
    $t = [Text.Encoding]::GetEncoding(28591).GetString([IO.File]::ReadAllBytes($f.FullName))
    foreach ($n in @("\Users\$env:USERNAME", "/Users/$env:USERNAME", "file:///")) {
        if ($t.IndexOf($n, [StringComparison]::OrdinalIgnoreCase) -ge 0) { throw "web app: '$n' found in $($f.Name)" }
    }
}
$size = (Get-ChildItem $dist -Recurse -File | Measure-Object Length -Sum).Sum / 1MB
Write-Host ("web app ready in server\internal\webapp\dist ({0:N1} MB before compression)" -f $size) -ForegroundColor Green
