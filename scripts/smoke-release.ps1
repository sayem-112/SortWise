param([string]$Version = "0.6.0", [int]$Port = 8878, [string]$CleanupPath = "")

$ErrorActionPreference = "Stop"
$workspace = Split-Path -Parent $PSScriptRoot
$exe = [IO.Path]::GetFullPath((Join-Path $workspace "release\Sortwise-v$Version\sortwise.exe"))
$archive = [IO.Path]::GetFullPath((Join-Path $workspace "release\Sortwise-v$Version-windows-x64.zip"))
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$cleanupTarget = if ($CleanupPath) { [IO.Path]::GetFullPath($CleanupPath) } else { "" }
if ($cleanupTarget) {
    if (-not $cleanupTarget.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($cleanupTarget) -notlike "sortwise-smoke-*") { throw "Refusing cleanup outside a verified smoke directory" }
    if (Test-Path -LiteralPath $cleanupTarget) { Remove-Item -LiteralPath $cleanupTarget -Recurse -Force }
}
$smoke = [IO.Path]::GetFullPath((Join-Path $tempRoot ("sortwise-smoke-" + [guid]::NewGuid().ToString("N"))))

if (-not $smoke.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) { throw "Smoke path escaped the temporary directory" }
if (-not (Test-Path -LiteralPath $exe)) { throw "Release executable not found" }
if (-not (Test-Path -LiteralPath $archive)) { throw "Release archive not found" }

New-Item -ItemType Directory -Path $smoke | Out-Null
$process = $null
try {
    $process = Start-Process -FilePath $exe -ArgumentList @("--no-open", "--port", "$Port", "--data-dir", $smoke) -PassThru -WindowStyle Hidden
    $health = $null
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        try {
            $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/v1/health" -TimeoutSec 1
            break
        } catch {
            Start-Sleep -Milliseconds 250
        }
    }
    if ($null -eq $health -or $health.status -ne "ok" -or $health.version -ne $Version) { throw "Packaged health check failed" }
    if (-not (Test-Path -LiteralPath (Join-Path $smoke "sortwise.db"))) { throw "Packaged application did not create its database" }
    $page = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/" -TimeoutSec 2
    if ($page.StatusCode -ne 200 -or $page.Content -notmatch "Sortwise") { throw "Embedded web application smoke check failed" }
    Write-Output ("Health: " + ($health | ConvertTo-Json -Compress))
} finally {
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        Wait-Process -Id $process.Id -Timeout 5 -ErrorAction SilentlyContinue
    }
    for ($cleanupAttempt = 0; $cleanupAttempt -lt 10 -and (Test-Path -LiteralPath $smoke); $cleanupAttempt++) {
        try { Remove-Item -LiteralPath $smoke -Recurse -Force -ErrorAction Stop } catch { Start-Sleep -Milliseconds 200 }
    }
    if (Test-Path -LiteralPath $smoke) { Write-Warning "Temporary smoke data remains at $smoke" }
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($archive)
try {
    $bad = $zip.Entries | Where-Object { $_.FullName -match "(test|fixture|package\.json|node_modules)" }
    if ($bad) { throw ("Development files found in release: " + (($bad.FullName) -join ", ")) }
    $requiredExtensionFiles = @("manifest.json", "popup.html", "popup.css", "popup-state.js", "popup.js", "service-worker.js", "sync-core.js", "page-hook.js", "x-bridge.js")
    foreach ($required in $requiredExtensionFiles) {
        if (-not ($zip.Entries | Where-Object { ($_.FullName -replace '\\','/') -like "*/extension/$required" })) { throw "Release extension is missing $required" }
    }
    Write-Output ("Release entries: " + $zip.Entries.Count)
} finally {
    $zip.Dispose()
}
