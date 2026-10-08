param([string]$Version = "0.6.0")

$ErrorActionPreference = "Stop"
$workspace = Split-Path -Parent $PSScriptRoot
$releaseRoot = Join-Path $workspace "release"
$staging = Join-Path $releaseRoot ("Sortwise-v" + $Version)
$archive = Join-Path $releaseRoot ("Sortwise-v" + $Version + "-windows-x64.zip")

npm --prefix $workspace run build
go -C $workspace test ./cmd/... ./internal/... ./migrations/... ./web
npm --prefix $workspace run test:ci

if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
New-Item -ItemType Directory -Path $staging | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "extension") | Out-Null

go -C $workspace build -trimpath -ldflags "-s -w -H=windowsgui -X sortwise/internal/buildinfo.Version=$Version" -o (Join-Path $staging "sortwise.exe") ./cmd/sortwise

$extensionFiles = @("manifest.json","popup.html","popup.css","popup-state.js","popup.js","service-worker.js","sync-core.js","page-hook.js","x-bridge.js")
foreach ($name in $extensionFiles) { Copy-Item -LiteralPath (Join-Path $workspace "extension\$name") -Destination (Join-Path $staging "extension\$name") }
Copy-Item -LiteralPath (Join-Path $workspace "extension\icons") -Destination (Join-Path $staging "extension\icons") -Recurse
foreach ($name in @("README.md")) { Copy-Item -LiteralPath (Join-Path $workspace $name) -Destination (Join-Path $staging $name) }

if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
Compress-Archive -Path $staging -DestinationPath $archive -CompressionLevel Optimal
Write-Output $archive
