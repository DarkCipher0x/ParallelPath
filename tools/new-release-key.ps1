# Creates ParallelPath's Android release signing key, once. Run from anywhere:  powershell -File tools\new-release-key.ps1
#
# - The keystore lives outside the project, under %LOCALAPPDATA%\ParallelPath-signing (never in the repo or dist).
# - Its password is random (32 bytes) and is only ever stored sealed to this Windows account with DPAPI; nobody types
#   it or sees it. tools\build-release.ps1 unseals it for the build.
# - Refuses to run if a key already exists: every update must be signed with the same key, so it is never replaced.
#   BACK THE FOLDER UP (both files, plus a DPAPI-free copy of the password somewhere safe, e.g. your password manager:
#   run tools\build-release.ps1 -ShowKeyPassword once). Without it, installed copies can't be updated.
# - Android's APK signature schemes only support RSA/EC keys - there is no post-quantum option yet. This signature only
#   proves who built the APK; everything the app protects uses ParallelPath's own post-quantum cryptography.
. "$PSScriptRoot\env.ps1"
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Security

$dir   = Join-Path $env:LOCALAPPDATA 'ParallelPath-signing'
$store = Join-Path $dir 'parallelpath-release.p12'
$pass  = Join-Path $dir 'parallelpath-release.pass.dpapi'
if ((Test-Path $store) -or (Test-Path $pass)) { throw "A release key already exists in $dir. It is never replaced." }
New-Item -ItemType Directory -Force $dir | Out-Null

$bytes = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
$secret = [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
$sealed = [Security.Cryptography.ProtectedData]::Protect([Text.Encoding]::UTF8.GetBytes($secret), $null, 'CurrentUser')
[IO.File]::WriteAllBytes($pass, $sealed)

# The password goes to keytool through an environment variable, never on the command line (visible to other processes).
$env:PP_KS_PASS = $secret
try {
    # keytool reports progress on stderr, which PowerShell would treat as an error: let cmd discard it.
    & cmd /c "keytool -genkeypair -keystore `"$store`" -storetype PKCS12 -alias parallelpath -keyalg RSA -keysize 4096 -validity 10950 -dname `"CN=DarkCipher Systems, O=DarkCipher Systems`" -storepass:env PP_KS_PASS -keypass:env PP_KS_PASS 2>nul"
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $store)) { Remove-Item $pass -ErrorAction SilentlyContinue; throw "keytool failed" }
} finally { Remove-Item Env:PP_KS_PASS }

# Only this Windows account may read the folder.
& icacls $dir /inheritance:r /grant:r "$($env:USERNAME):(OI)(CI)F" | Out-Null
Write-Host "Release key created in $dir" -ForegroundColor Green
Write-Host "Back up that folder now; updates need this exact key."
