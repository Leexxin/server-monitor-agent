[CmdletBinding()]
param([switch]$Purge,[switch]$Yes)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator privileges required.' }
if (-not [Environment]::Is64BitProcess) { throw 'Use 64-bit PowerShell.' }
if (-not $Yes) {
    $action = if ($Purge) {'Uninstall SMA and DELETE its configuration, token and agent logs'} else {'Uninstall SMA, preserving configuration, token and agent logs'}
    if ((Read-Host "$action ? Type YES") -cne 'YES') { Write-Host 'Cancelled.'; return }
}
$product = Get-ItemProperty 'HKLM:\Software\SMA' -ErrorAction SilentlyContinue
$log = Join-Path $env:TEMP ('sma-uninstall-{0}.log' -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
if ($product) {
    $code = $product.ProductCode
    if ($code -notmatch '^\{[0-9A-Fa-f-]{36}\}$') { throw 'Invalid SMA MSI product code.' }
    $process = Start-Process msiexec.exe -ArgumentList @('/x',$code,'/qn','/norestart','/L*v',('"{0}"' -f $log)) -Wait -PassThru
    if ($process.ExitCode -notin @(0,3010,1605)) { throw "Uninstall failed: $($process.ExitCode). See $log" }
} else { Write-Host 'SMA package is not installed.' }
Get-NetFirewallRule -Name 'SMA-Agent-Inbound' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
$data = Join-Path $env:ProgramData 'SMA'
if ($Purge -and (Test-Path -LiteralPath $data)) {
    if ((Get-Item -LiteralPath $data -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing to purge a redirected SMA data directory.' }
    Remove-Item -LiteralPath $data -Recurse -Force
    Write-Host 'Removed SMA configuration, token and agent logs. This deletion is not recoverable without your backup.'
} else { Write-Host "Configuration, token and logs preserved at: $data" }
Write-Host "Uninstall log: $log"
