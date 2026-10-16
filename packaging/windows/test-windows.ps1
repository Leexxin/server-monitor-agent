[CmdletBinding()]
param([switch]$Lifecycle,[string]$MsiPath,[string]$PreviousMsiPath)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$ctl = Join-Path $env:ProgramFiles 'SMA/sma-ctl.exe'
$data = Join-Path $env:ProgramData 'SMA'
function Invoke-Ctl([string[]]$Arguments) {
    & $ctl @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $($Arguments -join ' ')" }
}
if ($Lifecycle) {
    if ($env:SMA_RUN_WINDOWS_INSTALL_TESTS -ne '1') { throw 'Lifecycle test requires SMA_RUN_WINDOWS_INSTALL_TESTS=1 on a disposable machine.' }
    if ((Test-Path -LiteralPath $data) -or (Get-Service SMA -ErrorAction SilentlyContinue)) { throw 'Lifecycle test requires a clean machine without SMA.' }
    $oldHash = $null
    if ($PreviousMsiPath) {
        & "$PSScriptRoot/install-sma.ps1" -MsiPath $PreviousMsiPath
        $oldHash = (Get-FileHash -LiteralPath "$data/token").Hash
    }
    & "$PSScriptRoot/install-sma.ps1" -MsiPath $MsiPath -ListenAddress '0.0.0.0:9108' -AllowedSource '127.0.0.1'
    if ($oldHash -and (Get-FileHash -LiteralPath "$data/token").Hash -ne $oldHash) { throw 'Major upgrade changed bearer token.' }
    $rule = Get-NetFirewallRule -Name 'SMA-Agent-Inbound'
    $sources = @($rule | Get-NetFirewallAddressFilter | Select-Object -ExpandProperty RemoteAddress)
    if ($sources.Count -ne 1 -or $sources[0] -ne '127.0.0.1') { throw 'Firewall source scope was not applied.' }
}
Invoke-Ctl -Arguments @('doctor')
$config = Get-Content -Raw -LiteralPath "$data/config.json" | ConvertFrom-Json
$endpoint = $config.listenAddress -replace '^0\.0\.0\.0:', '127.0.0.1:' -replace '^\[::\]:', '[::1]:'
$base = "http://$endpoint"
$token = (Get-Content -Raw -LiteralPath $config.authTokenFile).Trim()
try {
    $null = Invoke-WebRequest "$base/v1/snapshot" -UseBasicParsing -TimeoutSec 10
    throw 'Unauthenticated snapshot must be rejected.'
} catch {
    if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 401) { throw }
}
$headers = @{Authorization="Bearer $token"}
$first = Invoke-RestMethod "$base/v1/snapshot" -Headers $headers -TimeoutSec 10
Start-Sleep -Seconds 2
$second = Invoke-RestMethod "$base/v1/snapshot" -Headers $headers -TimeoutSec 10
if ($second.host.os -ne 'windows' -or $second.host.architecture -ne 'amd64') { throw 'Incorrect platform metadata.' }
if ($second.cpu.logicalCount -lt 1 -or $second.memory.totalBytes -le 0 -or $second.filesystems.Count -lt 1 -or $second.uptimeSeconds -le 0) { throw 'Missing basic metrics.' }
if ($second.PSObject.Properties.Name -contains 'load') { throw 'Windows must not fabricate load averages.' }
$a = $first.cpu.seconds.total; $b = $second.cpu.seconds.total
$total = ($b.idle+$b.user+$b.system)-($a.idle+$a.user+$a.system)
if ($total -le 0) { throw 'CPU cumulative counters did not advance.' }
$usage = [math]::Round(100*(1-($b.idle-$a.idle)/$total),2)
if ($usage -lt 0 -or $usage -gt 100) { throw 'Invalid calculated CPU utilization.' }
$metrics = (Invoke-WebRequest "$base/metrics" -Headers $headers -UseBasicParsing -TimeoutSec 10).Content
if ($metrics -notmatch 'sma_cpu_seconds_total' -or $metrics -notmatch 'sma_memory_total_bytes') { throw 'Missing Prometheus metrics.' }
Write-Host "PASS: authenticated metrics; CPU $usage%; logical CPUs $($second.cpu.logicalCount); memory $($second.memory.totalBytes); volumes $($second.filesystems.Count)."
foreach ($warning in $second.errors) { Write-Warning "Collector $($warning.collector): $($warning.message)" }
if ($Lifecycle) {
    $gui = Join-Path $env:ProgramFiles 'SMA/sma-gui.exe'
    $menu = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'SMA'
    if (@(Get-ChildItem -LiteralPath $menu -Filter '*.lnk').Count -ne 1) { throw 'SMA Start menu entry is missing or duplicated.' }
    $guiOutput = if ($env:GITHUB_WORKSPACE) { Join-Path $env:GITHUB_WORKSPACE 'dist/gui-verification' } else { Join-Path $env:TEMP 'sma-gui-verification' }
    $env:SMA_GUI_TEST_MSI = $MsiPath
    $guiProcess = Start-Process -FilePath $gui -ArgumentList @('--self-test',('"{0}"' -f $guiOutput)) -PassThru
    if (-not $guiProcess.WaitForExit(90000)) { $guiProcess.Kill(); throw 'GUI self-test timed out.' }
    if ($guiProcess.ExitCode -ne 0) { throw "GUI verification failed; see $guiOutput" }
    Get-Content (Join-Path $guiOutput 'gui-result.txt')
    $network = Join-Path $env:ProgramFiles 'SMA/configure-network.ps1'
    & $network -ListenAddress '0.0.0.0:9108' -Sources '127.0.0.1'
    & $network -Read
    & $network -ListenAddress '127.0.0.1:9108'
    if (Get-NetFirewallRule -Name 'SMA-Agent-Inbound' -ErrorAction SilentlyContinue) { throw 'Local-only mode must remove the SMA inbound rule.' }
    & $network -ListenAddress '0.0.0.0:9108' -Sources '127.0.0.1'
    $initialHash = (Get-FileHash -LiteralPath $config.authTokenFile).Hash
    & "$PSScriptRoot/install-sma.ps1" -MsiPath $MsiPath
    if ((Get-FileHash -LiteralPath $config.authTokenFile).Hash -ne $initialHash) { throw 'Reinstallation changed token.' }
    Invoke-Ctl -Arguments @('disable'); Invoke-Ctl -Arguments @('enable'); Invoke-Ctl -Arguments @('stop'); Invoke-Ctl -Arguments @('start')
    try {
        $ErrorActionPreference = 'Continue'
        & $ctl config listen '192.0.2.1:9108' 2>$null
        $failedCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = 'Stop' }
    if ($failedCode -eq 0) { throw 'Unbindable address unexpectedly accepted.' }
    Invoke-Ctl -Arguments @('doctor')
    if ((Get-Content -Raw "$data/config.json" | ConvertFrom-Json).listenAddress -ne $config.listenAddress) { throw 'Configuration rollback failed.' }
    Invoke-Ctl -Arguments @('token','reset','--yes')
    try {
        $null = Invoke-WebRequest "$base/v1/snapshot" -Headers $headers -UseBasicParsing -TimeoutSec 10
        throw 'Old token remains valid.'
    } catch { if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 401) { throw } }
    $rotatedHash = (Get-FileHash -LiteralPath $config.authTokenFile).Hash
    & "$PSScriptRoot/uninstall-sma.ps1" -Yes
    if (-not (Test-Path "$data/config.json")) { throw 'Default uninstall deleted configuration.' }
    & "$PSScriptRoot/install-sma.ps1" -MsiPath $MsiPath
    if ((Get-FileHash -LiteralPath $config.authTokenFile).Hash -ne $rotatedHash) { throw 'Reinstall did not retain token.' }
    & "$PSScriptRoot/uninstall-sma.ps1" -Purge -Yes
    if ((Test-Path $data) -or (Get-Service SMA -ErrorAction SilentlyContinue)) { throw 'Purge did not clean installation.' }
    if (Get-NetFirewallRule -Name 'SMA-Agent-Inbound' -ErrorAction SilentlyContinue) { throw 'Uninstall left its firewall rule.' }
    Write-Host 'PASS: install/reinstall, autostart configuration, service controls, config rollback, token rotation, preserved uninstall, reinstall and purge.'
}
