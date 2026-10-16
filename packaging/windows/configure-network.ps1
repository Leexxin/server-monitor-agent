[CmdletBinding()]
param([string]$ListenAddress,[string]$Sources,[switch]$Read)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$ruleName = 'SMA-Agent-Inbound'
$rule = Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue
if ($Read) {
    if ($rule) { $filter = $rule | Get-NetFirewallAddressFilter; @{ enabled = [string]$rule.Enabled; sources = @($filter.RemoteAddress) } | ConvertTo-Json -Compress }
    else { @{ enabled = 'False'; sources = @() } | ConvertTo-Json -Compress }
    return
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator privileges required.' }
if ($ListenAddress -notmatch '^(\[[0-9a-fA-F:]+\]|[0-9.]+):([0-9]+)$') { throw 'ListenAddress must be an IP address and port.' }
$port = [int]$Matches[2]
if ($port -lt 1 -or $port -gt 65535) { throw 'Port must be 1..65535.' }
$listenHost = $ListenAddress.Substring(0,$ListenAddress.LastIndexOf(':')).Trim('[',']')
$ip = $null
if (-not [Net.IPAddress]::TryParse($listenHost,[ref]$ip)) { throw 'Invalid listen IP address.' }
$localOnly = [Net.IPAddress]::IsLoopback($ip)
$addresses = @($Sources -split ',' | ForEach-Object {$_.Trim()} | Where-Object {$_})
if (-not $localOnly -and $addresses.Count -eq 0) { throw 'Specify the SMA Center IP address or a restricted CIDR for remote access.' }
foreach ($address in $addresses) {
    $parts = $address.Split('/'); $remoteIP = $null
    if ($parts.Count -gt 2 -or -not [Net.IPAddress]::TryParse($parts[0],[ref]$remoteIP)) { throw "Invalid allowed source: $address" }
    if ($parts.Count -eq 2) {
        $prefix = 0; $max = if ($remoteIP.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetwork) {32} else {128}
        if (-not [int]::TryParse($parts[1],[ref]$prefix) -or $prefix -lt 1 -or $prefix -gt $max) { throw "Invalid CIDR: $address" }
    }
}
$previousListen = (Get-Content -Raw -LiteralPath (Join-Path $env:ProgramData 'SMA/config.json') | ConvertFrom-Json).listenAddress
$previousRule = $null
if ($rule) {
    $oldPort = $rule | Get-NetFirewallPortFilter; $oldAddress = $rule | Get-NetFirewallAddressFilter; $oldApp = $rule | Get-NetFirewallApplicationFilter
    $previousRule = @{Enabled=$rule.Enabled;Direction=$rule.Direction;Action=$rule.Action;Profile=$rule.Profile;Protocol=$oldPort.Protocol;LocalPort=$oldPort.LocalPort;RemoteAddress=$oldAddress.RemoteAddress;Program=$oldApp.Program}
}
$ctl = Join-Path $env:ProgramFiles 'SMA/sma-ctl.exe'
& $ctl config listen $ListenAddress
if ($LASTEXITCODE -ne 0) { throw 'Could not apply listen address; see the service log.' }
try {
    if ($localOnly) { if ($rule) { $rule | Remove-NetFirewallRule } }
    else {
        $settings = @{Enabled='True';Direction='Inbound';Action='Allow';Profile='Any';Protocol='TCP';LocalPort=$port;RemoteAddress=$addresses;Program=(Join-Path $env:ProgramFiles 'SMA/sma.exe')}
        if ($rule) { Set-NetFirewallRule -Name $ruleName @settings | Out-Null }
        else { New-NetFirewallRule -Name $ruleName -DisplayName 'SMA Agent inbound' @settings | Out-Null }
    }
    Write-Output 'Connection settings and scoped firewall saved.'
} catch {
    $failure = $_
    try {
        & $ctl config listen $previousListen
        if ($LASTEXITCODE -ne 0) { throw 'Listener rollback failed.' }
        if ($previousRule) { Set-NetFirewallRule -Name $ruleName @previousRule | Out-Null }
        else { Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule }
    } catch { throw "Network change failed: $($failure.Exception.Message). Rollback also failed: $($_.Exception.Message)" }
    throw $failure
}
