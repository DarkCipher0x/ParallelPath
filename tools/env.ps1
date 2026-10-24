# Dot-source this (". .\tools\env.ps1") to put the shared portable toolchain on PATH for this shell only.
# Nothing needs admin: Go, the JDK, Gradle and the Android SDK live under %LOCALAPPDATA% (shared with other projects).
$tc = Join-Path $env:LOCALAPPDATA 'DcxToolchain'
$env:GOROOT           = Join-Path $tc 'go'
$env:GOPATH           = Join-Path $tc 'gopath'
$env:GOTOOLCHAIN      = 'local'
$env:GOFLAGS          = '-trimpath'
$env:JAVA_HOME        = Join-Path $tc 'jdk21'
$env:ANDROID_HOME     = Join-Path $env:LOCALAPPDATA 'Android\Sdk'
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:Path = @(
    (Join-Path $env:GOROOT 'bin'),
    (Join-Path $env:GOPATH 'bin'),
    (Join-Path $env:JAVA_HOME 'bin'),
    (Join-Path $env:ANDROID_HOME 'platform-tools'),
    $env:Path
) -join ';'
