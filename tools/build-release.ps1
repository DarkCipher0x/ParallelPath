# Builds every ParallelPath download from source into dist\<version>\ (the version is in the VERSION file):
#
#   ParallelPath-Setup-<v>.exe                     Windows app setup (per user, no admin)
#   ParallelPath-Server-Setup-<v>.exe              Windows server setup (admin; service, firewall, backups)
#   ParallelPath-Server-<v>-linux-x64.tar.gz       Ubuntu server: sudo sh install.sh
#   ParallelPath-<v>-android.apk                   Android app for phones (arm64), signed with the release key
#   ParallelPath-<v>-android-emulator.apk          the same for x86_64 emulators
#   SHA256SUMS.txt, android-signing-certificate.txt
#
# Run from anywhere:  powershell -File tools\build-release.ps1 [-SkipTests] [-SkipAndroid] [-Force]
# -Force replaces an existing dist\<version> (test builds only - a published version is never rebuilt).
param([switch]$SkipTests, [switch]$SkipAndroid, [switch]$Force, [switch]$ShowKeyPassword)

. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$version = ([IO.File]::ReadAllText("$root\VERSION")).Trim()
$out = Join-Path $root "dist\$version"
$stage = Join-Path $root "dist\.stage-$version"
Add-Type -AssemblyName System.Security, System.IO.Compression.FileSystem

function Step($text) { Write-Host "`n== $text" -ForegroundColor Cyan }
function Run($what) { & $what; if ($LASTEXITCODE -ne 0) { throw "failed: $what" } }
function Sha256($file) { (Get-FileHash $file -Algorithm SHA256).Hash.ToLower() }
function WriteHash($file) { [IO.File]::WriteAllText("$file.sha256", "$(Sha256 $file)  $(Split-Path $file -Leaf)`n") }

# ---- Android signing key (DPAPI-sealed password; see tools\new-release-key.ps1)
$keyDir = Join-Path $env:LOCALAPPDATA 'ParallelPath-signing'
$store = Join-Path $keyDir 'parallelpath-release.p12'
$passFile = Join-Path $keyDir 'parallelpath-release.pass.dpapi'
if (-not $SkipAndroid) {
    if (-not (Test-Path $store)) { throw "No Android release key. Run tools\new-release-key.ps1 first (once, ever)." }
    $secret = [Text.Encoding]::UTF8.GetString([Security.Cryptography.ProtectedData]::Unprotect([IO.File]::ReadAllBytes($passFile), $null, 'CurrentUser'))
    if ($ShowKeyPassword) { Write-Host "Keystore password (store it in your password manager): $secret"; return }
}

$gradleAndroid = [IO.File]::ReadAllText("$root\clients\apps\android\build.gradle.kts")
if ($gradleAndroid -notmatch "versionName = `"$([regex]::Escape($version))`"") { throw "clients\apps\android\build.gradle.kts versionName doesn't match VERSION ($version)" }
if ((Test-Path $out) -and -not $Force) { throw "dist\$version already exists. Bump VERSION (or -Force for a test build)." }

# The in-app help answers from the published guide and FAQ: refresh its copy first (a test checks they match).
Copy-Item "$root\docs\USER-GUIDE.md", "$root\docs\FAQ.md" "$root\engine\help\kb\" -Force

# ---- 1. Tests
if (-not $SkipTests) {
    Step "Go tests"
    foreach ($m in 'core', 'server', 'engine') { Push-Location "$root\$m"; try { Run { go test ./... } } finally { Pop-Location } }
}

# ---- 2. The web app (built first: ppserver embeds it)
Step "Web app"
Run { powershell -NoProfile -ExecutionPolicy Bypass -File "$PSScriptRoot\build-web.ps1" -Version $version }

# ---- 3. Go programs
Step "Go programs ($version)"
$ld = "-s -w -buildid= -X main.version=$version"
New-Item -ItemType Directory -Force "$root\bin" | Out-Null
Push-Location "$root\server"
try {
    Run { go build -ldflags $ld -o "$root\bin\ppserver.exe" ./cmd/ppserver }
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Run { go build -ldflags $ld -o "$root\bin\ppserver-linux-amd64" ./cmd/ppserver }
} finally { Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue; Pop-Location }
Push-Location "$root\engine"; try { Run { go build -ldflags $ld -o "$root\bin\ppengine.exe" ./cmd/ppengine } } finally { Pop-Location }
Push-Location "$root\installer\windows\uninstall"; try { Run { go build -ldflags "$ld -H windowsgui" -o "$root\bin\ppuninstall.exe" . } } finally { Pop-Location }
Copy-Item "$root\bin\ppengine.exe" "$root\clients\apps\desktop\resources\windows\ppengine.exe" -Force

if (-not $SkipAndroid) {
    Step "Android engine library (gomobile)"
    $env:ANDROID_NDK_HOME = (Get-ChildItem "$env:ANDROID_HOME\ndk" -Directory | Sort-Object { [version]$_.Name } | Select-Object -Last 1).FullName
    $env:GOWORK = 'off'
    Push-Location "$root\engine"
    try { Run { gomobile bind '-target=android/arm64,android/amd64' -androidapi 29 '-javapkg=systems.darkcipher.parallelpath.gocore' -trimpath '-ldflags=-s -w' -o "$root\clients\libs\ppengine.aar" ./mobile } }
    finally { Remove-Item Env:GOWORK -ErrorAction SilentlyContinue; Pop-Location }
}

# ---- 4. Apps (Compose)
Step "Windows app$(if (-not $SkipAndroid) { ' and Android app' })"
$tasks = @(':apps:desktop:createDistributable')
if (-not $SkipAndroid) {
    $tasks += ':apps:android:assembleRelease'
    $env:PP_KEYSTORE = $store; $env:PP_KEYSTORE_PASS = $secret; $env:PP_KEY_ALIAS = 'parallelpath'; $env:PP_KEY_PASS = $secret
}
# A fresh app image every time: Gradle has been seen to keep an older bundled engine.
Remove-Item -Recurse -Force "$root\clients\apps\desktop\build\compose\binaries" -ErrorAction SilentlyContinue
Push-Location "$root\clients"
try { Run { .\gradlew.bat @tasks --console=plain -q } }
finally { 'PP_KEYSTORE', 'PP_KEYSTORE_PASS', 'PP_KEY_ALIAS', 'PP_KEY_PASS' | ForEach-Object { Remove-Item "Env:$_" -ErrorAction SilentlyContinue }; Pop-Location }

# ---- 5. Privacy gate: nothing that ships may contain local paths or this account's name
Step "Privacy gate"
$image = "$root\clients\apps\desktop\build\compose\binaries\main\app\ParallelPath"
if ((Sha256 "$image\app\resources\ppengine.exe") -ne (Sha256 "$root\bin\ppengine.exe")) { throw "the app image doesn't carry the engine just built" }
# The bare account name only as a word of its own: compression dictionaries (brotli's, inside the bundled skiko wasm)
# hold ordinary English words that contain it.
$u = [regex]::Escape($env:USERNAME)
$needles = @([regex]::Escape("\Users\$env:USERNAME"), [regex]::Escape("/Users/$env:USERNAME"), "(?<![a-z])$u(?![a-z])") |
    ForEach-Object { New-Object regex $_, 'IgnoreCase' }
$check = @(Get-ChildItem $image -Recurse -File | Where-Object { $_.FullName -notmatch '\\runtime\\' }) +
         @(Get-Item "$root\bin\ppserver.exe", "$root\bin\ppserver-linux-amd64", "$root\bin\ppengine.exe", "$root\bin\ppuninstall.exe")
foreach ($f in $check) {
    $text = [Text.Encoding]::GetEncoding(28591).GetString([IO.File]::ReadAllBytes($f.FullName))
    foreach ($n in $needles) { if ($n.IsMatch($text)) { throw "privacy gate: '$n' found in $($f.FullName)" } }
}
Write-Host "clean ($($check.Count) files)"

# ---- 6. Setup payloads and the Windows setups
Step "Windows setups"
Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage, "$root\installer\windows\PPSetup.Server\Payload", "$root\installer\windows\PPSetup.App\Payload" | Out-Null
Copy-Item "$root\bin\ppserver.exe" "$root\installer\windows\PPSetup.Server\Payload\ppserver.exe" -Force
WriteHash "$root\installer\windows\PPSetup.Server\Payload\ppserver.exe"
$zip = "$root\installer\windows\PPSetup.App\Payload\app.zip"
Remove-Item $zip -ErrorAction SilentlyContinue
[IO.Compression.ZipFile]::CreateFromDirectory($image, $zip, [IO.Compression.CompressionLevel]::Optimal, $true)
WriteHash $zip
Copy-Item "$root\bin\ppuninstall.exe" "$root\installer\windows\PPSetup.App\Payload\Uninstall ParallelPath.exe" -Force
WriteHash "$root\installer\windows\PPSetup.App\Payload\Uninstall ParallelPath.exe"
$props = "-p:InformationalVersion=$version", "-p:Version=$(($version -split '-')[0])"
Push-Location "$root\installer\windows"
try {
    Run { dotnet publish PPSetup.Server -c Release -nologo -v q @props -o "$stage\server-setup" }
    Run { dotnet publish PPSetup.App -c Release -nologo -v q @props -o "$stage\app-setup" }
} finally { Pop-Location }

# ---- 7. Linux server bundle
Step "Linux server bundle"
$lin = "$stage\ParallelPath-Server-$version-linux-x64"
New-Item -ItemType Directory -Force $lin | Out-Null
Copy-Item "$root\bin\ppserver-linux-amd64" "$lin\ppserver"
Copy-Item "$root\installer\linux\install.sh", "$root\installer\linux\README.txt", "$root\LICENSE", "$root\docs\ADMIN-GUIDE.md" $lin
# LF line endings for the shell files, whatever git or an editor did.
foreach ($f in "$lin\install.sh", "$lin\README.txt") { [IO.File]::WriteAllText($f, ([IO.File]::ReadAllText($f) -replace "`r`n", "`n")) }
[IO.File]::WriteAllText("$lin\SHA256SUMS", "$(Sha256 "$lin\ppserver")  ppserver`n$(Sha256 "$lin\install.sh")  install.sh`n")
Push-Location $stage
# Packed by our own tool: tar on Windows can't record Unix permissions (it would ship world-writable files).
try { Run { go run "$root\installer\linux\mktar" "ParallelPath-Server-$version-linux-x64.tar.gz" "ParallelPath-Server-$version-linux-x64" } } finally { Pop-Location }

# ---- 8. Collect
Step "dist\$version"
Remove-Item -Recurse -Force $out -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $out | Out-Null
Copy-Item "$stage\app-setup\ParallelPath-Setup.exe" "$out\ParallelPath-Setup-$version.exe"
Copy-Item "$stage\server-setup\ParallelPath-Server-Setup.exe" "$out\ParallelPath-Server-Setup-$version.exe"
Copy-Item "$stage\ParallelPath-Server-$version-linux-x64.tar.gz" $out
if (-not $SkipAndroid) {
    $tools = Get-ChildItem "$env:ANDROID_HOME\build-tools" -Directory | Sort-Object { [version]$_.Name } | Select-Object -Last 1
    $apks = @{ 'arm64-v8a' = "ParallelPath-$version-android.apk"; 'x86_64' = "ParallelPath-$version-android-emulator.apk" }
    foreach ($abi in $apks.Keys) {
        $src = "$root\clients\apps\android\build\outputs\apk\release\android-$abi-release.apk"
        $certs = & cmd /c "`"$($tools.FullName)\apksigner.bat`" verify --print-certs `"$src`" 2>nul"
        if ($LASTEXITCODE -ne 0) { throw "APK signature check failed ($abi)" }
        $cert = ($certs | Select-String 'SHA-256 digest: (\w+)' | Select-Object -First 1).Matches[0].Groups[1].Value
        Copy-Item $src "$out\$($apks[$abi])"
    }
    [IO.File]::WriteAllText("$out\android-signing-certificate.txt", "SHA-256 of the ParallelPath Android signing certificate:`n$cert`n")
}
Copy-Item "$root\installer\TESTING.md" "$out\TESTING.md"

# The guides, as Markdown and as PDF.
& "$PSScriptRoot\build-guides.ps1" -Out $out
$sums = Get-ChildItem $out -File -Recurse | Where-Object Name -ne 'SHA256SUMS.txt' | Sort-Object FullName |
    ForEach-Object { "$(Sha256 $_.FullName)  $($_.FullName.Substring($out.Length + 1).Replace('\', '/'))" }
[IO.File]::WriteAllText("$out\SHA256SUMS.txt", ($sums -join "`n") + "`n")
# Edge's helper processes can outlive the print a moment; they hold the throwaway profile open.
Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" | Where-Object { $_.CommandLine -like "*$stage*" } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 1
Remove-Item -Recurse -Force $stage
Get-ChildItem $out | Select-Object Name, @{ n = 'MB'; e = { [math]::Round($_.Length / 1MB, 1) } } | Format-Table -AutoSize
Write-Host "Built ParallelPath $version" -ForegroundColor Green
