# Builds bin/release/v<version>/ with the installer, the portable zip and RELEASE_NOTES.md (from CHANGELOG.md).
param(
    [string]$Makensis = $env:MAKENSIS,
    [string]$SignThumbprint = $env:SIGN_CERT_THUMBPRINT,
    [string]$SignPfx = $env:SIGN_PFX
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$version = (Get-Content -Raw -LiteralPath "$root/VERSION").Trim()

& "$root/build.ps1" -Makensis $Makensis -SignThumbprint $SignThumbprint -SignPfx $SignPfx
if (-not $?) { throw '构建失败' }

$out = "$root/bin/release/v$version"
New-Item -ItemType Directory -Force -Path $out | Out-Null
$artifacts = @("session-desk-windows-amd64-v$version.zip", "session-desk-$version-setup.exe") | Where-Object { Test-Path -LiteralPath "$root/bin/$_" }
foreach ($name in $artifacts) { Copy-Item -LiteralPath "$root/bin/$name" -Destination $out -Force }

$changelog = Get-Content -LiteralPath "$root/CHANGELOG.md" -Encoding UTF8
$start = ($changelog | Select-String -Pattern "^## \[?v?$([regex]::Escape($version))\]?" | Select-Object -First 1).LineNumber
if ($start) {
    $rest = $changelog[$start..($changelog.Count - 1)]
    $end = ($rest | Select-String -Pattern '^## ' | Select-Object -First 1).LineNumber
    $notes = $(if ($end) { $rest[0..($end - 2)] } else { $rest }) | Where-Object { $_ -notmatch '^\[[^\]]+\]: ' }
    [IO.File]::WriteAllText("$out/RELEASE_NOTES.md", (($notes -join "`n").Trim() + "`n"), [Text.UTF8Encoding]::new($false))
} else {
    Write-Warning "CHANGELOG.md 中没有 $version 的条目"
}
Get-ChildItem -LiteralPath $out | Select-Object Name, Length
