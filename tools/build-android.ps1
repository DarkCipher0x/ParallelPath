# Builds the Android app into dist\<version>\: the Go engine library (gomobile), the signed release APKs (phones and
# x86_64 emulators) and the signing certificate's fingerprint. build-release.ps1 runs it; run it alone to rebuild
# only the Android downloads.
#
# gomobile writes the engine's module replace paths (pp/core => <folder>) into the library's build info as absolute
# paths, which -trimpath doesn't touch. So the engine is built from a temporary drive letter mapped to the project
# (subst), and every file inside each APK is scanned for local paths and the account name before it's copied out.
#
#   powershell -File tools\build-android.ps1 [-Out dist\<version>]
param([string]$Out = '')
. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$version = ([IO.File]::ReadAllText("$root\VERSION")).Trim()
if (-not $Out) { $Out = Join-Path $root "dist\$version" }
Add-Type -AssemblyName System.Security, System.IO.Compression, System.IO.Compression.FileSystem
function Run($what) { & $what; if ($LASTEXITCODE -ne 0) { throw "failed: $what" } }

# The release key (DPAPI-sealed password; see tools\new-release-key.ps1). Never replaced.
$keyDir = Join-Path $env:LOCALAPPDATA 'ParallelPath-signing'
$store = Join-Path $keyDir 'parallelpath-release.p12'
if (-not (Test-Path $store)) { throw 'No Android release key. Run tools\new-release-key.ps1 first (once, ever).' }
$secret = [Text.Encoding]::UTF8.GetString([Security.Cryptography.ProtectedData]::Unprotect(
    [IO.File]::ReadAllBytes((Join-Path $keyDir 'parallelpath-release.pass.dpapi')), $null, 'CurrentUser'))

# ---- the engine library, built from a neutral drive letter
$used = (Get-PSDrive -PSProvider FileSystem).Name
$letter = @('P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W') | Where-Object { $_ -notin $used } | Select-Object -First 1
if (-not $letter) { throw 'no free drive letter for the build' }
& subst.exe "${letter}:" $root
if ($LASTEXITCODE) { throw 'subst failed' }
try {
    $env:ANDROID_NDK_HOME = (Get-ChildItem "$env:ANDROID_HOME\ndk" -Directory | Sort-Object { [version]$_.Name } | Select-Object -Last 1).FullName
    $env:GOWORK = 'off'
    Push-Location "${letter}:\engine"
    try { Run { gomobile bind '-target=android/arm64,android/amd64' -androidapi 29 '-javapkg=systems.darkcipher.parallelpath.gocore' -trimpath '-ldflags=-s -w -buildid=' -o "${letter}:\clients\libs\ppengine.aar" ./mobile } }
    finally { Remove-Item Env:GOWORK -ErrorAction SilentlyContinue; Pop-Location }
} finally { & subst.exe "${letter}:" /D | Out-Null }

# ---- the signed APKs
$env:PP_KEYSTORE = $store; $env:PP_KEYSTORE_PASS = $secret; $env:PP_KEY_ALIAS = 'parallelpath'; $env:PP_KEY_PASS = $secret
Push-Location "$root\clients"
try { Run { .\gradlew.bat ':apps:android:assembleRelease' --console=plain -q } }
finally { 'PP_KEYSTORE', 'PP_KEYSTORE_PASS', 'PP_KEY_ALIAS', 'PP_KEY_PASS' | ForEach-Object { Remove-Item "Env:$_" -ErrorAction SilentlyContinue }; Pop-Location }

# ---- privacy gate over every file inside each APK (the native library included)
$u = [regex]::Escape($env:USERNAME)
$terms = @([regex]::Escape("\Users\$env:USERNAME"), [regex]::Escape("/Users/$env:USERNAME"), "(?<![a-z])$u(?![a-z])")
$termsFile = Join-Path $env:LOCALAPPDATA 'ParallelPath-release\privacy-terms.txt'
if (Test-Path $termsFile) { $terms += @([IO.File]::ReadAllLines($termsFile) | Where-Object { $_.Trim() -and -not $_.StartsWith('#') }) }
$gate = $terms | ForEach-Object { New-Object regex $_, 'IgnoreCase' }
$apks = [ordered]@{ 'arm64-v8a' = "ParallelPath-$version-android.apk"; 'x86_64' = "ParallelPath-$version-android-emulator.apk" }
$tools = Get-ChildItem "$env:ANDROID_HOME\build-tools" -Directory | Sort-Object { [version]$_.Name } | Select-Object -Last 1
New-Item -ItemType Directory -Force $Out | Out-Null
foreach ($abi in $apks.Keys) {
    $src = "$root\clients\apps\android\build\outputs\apk\release\android-$abi-release.apk"
    $zip = [IO.Compression.ZipFile]::OpenRead($src)
    try {
        foreach ($e in $zip.Entries) {
            if ($e.Length -eq 0) { continue }
            $ms = New-Object IO.MemoryStream
            $s = $e.Open(); $s.CopyTo($ms); $s.Dispose()
            $bytes = $ms.ToArray()
            $text = [Text.Encoding]::GetEncoding(28591).GetString($bytes) + "`n" + [Text.Encoding]::Unicode.GetString($bytes)
            # Compiled Java (.dex) is the framework's and libraries' code plus ours, whose source passed the full gate:
            # it gets the path and account-name checks; the personal terms (common words among them) cover everything else.
            $use = if ($e.FullName -like '*.dex') { $gate[0..2] } else { $gate }
            foreach ($g in $use) { if ($g.IsMatch($text)) { throw "privacy gate: '$g' inside $($apks[$abi]) -> $($e.FullName)" } }
        }
    } finally { $zip.Dispose() }
    $certs = & cmd /c "`"$($tools.FullName)\apksigner.bat`" verify --print-certs `"$src`" 2>nul"
    if ($LASTEXITCODE -ne 0) { throw "APK signature check failed ($abi)" }
    $cert = ($certs | Select-String 'SHA-256 digest: (\w+)' | Select-Object -First 1).Matches[0].Groups[1].Value
    Copy-Item $src (Join-Path $Out $apks[$abi]) -Force
    Write-Host "$($apks[$abi]): every file inside is clean; signed $($cert.Substring(0, 16))..."
}
[IO.File]::WriteAllText((Join-Path $Out 'android-signing-certificate.txt'), "SHA-256 of the ParallelPath Android signing certificate:`n$cert`n")
