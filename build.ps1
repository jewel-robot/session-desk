param(
    # Path to makensis.exe. Defaults to $env:MAKENSIS, makensis on PATH, or a portable copy under %LOCALAPPDATA%\SessionDeskBuildTools.
    [string]$Makensis = $env:MAKENSIS,
    [switch]$SkipInstaller,
    # Optional Authenticode signing. Either a certificate thumbprint from the CurrentUser\My store
    # or a .pfx file (password from $env:SIGN_PFX_PASSWORD). Unsigned when neither is set.
    [string]$SignThumbprint = $env:SIGN_CERT_THUMBPRINT,
    [string]$SignPfx = $env:SIGN_PFX,
    [string]$TimestampUrl = $(if ($env:SIGN_TIMESTAMP_URL) { $env:SIGN_TIMESTAMP_URL } else { 'http://timestamp.digicert.com' })
)
$ErrorActionPreference = 'Stop'

$version = (Get-Content -Raw -LiteralPath "$PSScriptRoot/VERSION").Trim()
if ($version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw "VERSION 文件格式无效：$version" }
$numericVersion = ($version -split '-')[0] + '.0'
$commit = (git -C "$PSScriptRoot" rev-parse --short HEAD 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $commit) { $commit = 'unknown' }
$dirty = (git -C "$PSScriptRoot" status --porcelain 2>$null)
if ($dirty) { $commit = "$commit-dirty" }
$date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
# REPOSITORY holds the GitHub "owner/name" used for update checks and links. Empty disables both.
$repository = ''
if (Test-Path -LiteralPath "$PSScriptRoot/REPOSITORY") { $repository = (Get-Content -Raw -LiteralPath "$PSScriptRoot/REPOSITORY").Trim() }
if ($repository -and $repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw "REPOSITORY 文件格式无效：$repository" }
$ldVersion = "-X sessiondesk/internal/buildinfo.Version=$version -X sessiondesk/internal/buildinfo.Commit=$commit -X sessiondesk/internal/buildinfo.Date=$date -X sessiondesk/internal/buildinfo.Repository=$repository"
Write-Output "Session Desk $version ($commit) $repository"

$signing = [bool]($SignThumbprint -or $SignPfx)
$signtool = $null
if ($signing) {
    $signtool = (Get-Command signtool.exe -ErrorAction SilentlyContinue).Source
    if (-not $signtool) {
        $signtool = Get-ChildItem -Path "${env:ProgramFiles(x86)}/Windows Kits/10/bin" -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\' } | Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName
    }
    if (-not $signtool) { throw '已要求签名，但未找到 signtool.exe（Windows SDK）' }
}
function Invoke-Sign([string]$Path) {
    if (-not $signing) { return }
    $arguments = @('sign', '/fd', 'sha256', '/tr', $TimestampUrl, '/td', 'sha256', '/d', 'Session Desk')
    if ($SignThumbprint) { $arguments += @('/sha1', $SignThumbprint) } else { $arguments += @('/f', $SignPfx); if ($env:SIGN_PFX_PASSWORD) { $arguments += @('/p', $env:SIGN_PFX_PASSWORD) } }
    & $signtool @arguments $Path
    if ($LASTEXITCODE -ne 0) { throw "签名失败：$Path" }
    $status = (Get-AuthenticodeSignature -LiteralPath $Path).Status
    Write-Output "已签名 $(Split-Path -Leaf $Path)：$status"
}

Push-Location -LiteralPath "$PSScriptRoot/frontend"
try { npm ci --no-audit --no-fund } finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw '安装前端依赖失败' }

Push-Location -LiteralPath "$PSScriptRoot/frontend"
try { npm run build } finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw '构建前端失败' }

go -C "$PSScriptRoot" test ./...
if ($LASTEXITCODE -ne 0) { throw 'Go 测试失败' }
go -C "$PSScriptRoot" vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go 静态检查失败' }

# Windows resources: app icon (group #3, the ID Wails loads for the window and taskbar), version info and a per-monitor DPI manifest.
Push-Location $PSScriptRoot
try {
    go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64 --product-version $numericVersion --file-version $numericVersion
    if ($LASTEXITCODE -ne 0) { throw '生成 Windows 资源失败' }
} finally {
    Pop-Location
}

New-Item -ItemType Directory -Force -Path "$PSScriptRoot/bin" | Out-Null

$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
$previousCgo = $env:CGO_ENABLED
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    # -H windowsgui marks the exe as a GUI program; without it Windows opens a console window next to the app.
    go -C "$PSScriptRoot" build -trimpath -ldflags "-s -w -H windowsgui $ldVersion" -o "$PSScriptRoot/bin/session-desk.exe" .
    if ($LASTEXITCODE -ne 0) { throw 'Windows 应用构建失败' }
    $env:GOOS = 'linux'
    go -C "$PSScriptRoot" build -trimpath -ldflags "-s -w $ldVersion" -o "$PSScriptRoot/bin/session-desk-worker" ./cmd/session-desk-worker
    if ($LASTEXITCODE -ne 0) { throw 'WSL 程序构建失败' }
} finally {
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    $env:CGO_ENABLED = $previousCgo
}

# Guard against console builds: PE optional header subsystem must be 2 (Windows GUI), not 3 (console).
$bytes = [IO.File]::ReadAllBytes("$PSScriptRoot/bin/session-desk.exe")
$peOffset = [BitConverter]::ToInt32($bytes, 0x3C)
$subsystem = [BitConverter]::ToUInt16($bytes, $peOffset + 24 + 68)
if ($subsystem -ne 2) { throw "session-desk.exe 的 PE 子系统是 $subsystem，应为 2（GUI），否则启动时会弹出终端窗口" }
Invoke-Sign "$PSScriptRoot/bin/session-desk.exe"

node "$PSScriptRoot/scripts/notices.mjs"
if ($LASTEXITCODE -ne 0) { throw '收集许可证失败' }

$zipName = "session-desk-windows-amd64-v$version.zip"
$package = "$PSScriptRoot/bin/$zipName"
Compress-Archive -Path "$PSScriptRoot/bin/session-desk.exe", "$PSScriptRoot/bin/session-desk-worker", "$PSScriptRoot/bin/LICENSE.txt", "$PSScriptRoot/bin/THIRD_PARTY_NOTICES.md", "$PSScriptRoot/bin/licenses" -DestinationPath $package -Force
Write-Output $package

# Per-user NSIS installer (no administrator rights). Skipped with a note when makensis is not available.
if ($SkipInstaller) { return }
if (-not $Makensis) {
    $command = Get-Command makensis -ErrorAction SilentlyContinue
    if ($command) { $Makensis = $command.Source }
}
if (-not $Makensis) {
    $portable = Get-ChildItem -Path "$env:LOCALAPPDATA/SessionDeskBuildTools" -Filter makensis.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($portable) { $Makensis = $portable.FullName }
}
if (-not $Makensis -or -not (Test-Path -LiteralPath $Makensis)) {
    Write-Warning '未找到 makensis（NSIS 3），跳过安装程序。安装 NSIS 或通过 -Makensis / $env:MAKENSIS 指定路径后重试。'
    return
}
$setupName = "session-desk-$version-setup.exe"
$nsisArguments = @('/V2', '/INPUTCHARSET', 'UTF8', "/DVERSION=$version", "/DVERSION4=$numericVersion", "/DBIN=$PSScriptRoot\bin", "/DOUTFILE=$PSScriptRoot\bin\$setupName")
if ($repository) { $nsisArguments += "/DWEBSITE=https://github.com/$repository" }
# The uninstaller is written during compilation; NSIS signs it through !uninstfinalize (certificate store thumbprint only).
if ($signing -and $SignThumbprint) { $nsisArguments += @("/DSIGNTOOL=$signtool", "/DSIGN_SHA1=$SignThumbprint", "/DSIGN_TSA=$TimestampUrl") }
& $Makensis @nsisArguments "$PSScriptRoot\installer\session-desk.nsi"
if ($LASTEXITCODE -ne 0) { throw '安装程序构建失败' }
$setup = "$PSScriptRoot/bin/$setupName"
Invoke-Sign $setup
Write-Output $setup
