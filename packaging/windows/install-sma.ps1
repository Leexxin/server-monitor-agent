[CmdletBinding()]
param(
    [string]$MsiPath,
    [string]$ListenAddress,
    [string[]]$AllowedSource,
    [switch]$ShowToken
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Open 64-bit PowerShell as Administrator, then run this installer again.'
}
if (-not [Environment]::Is64BitProcess) { throw 'Use 64-bit PowerShell.' }
$data = Join-Path $env:ProgramData 'SMA'
$ctl = Join-Path $env:ProgramFiles 'SMA/sma-ctl.exe'
$installLog = Join-Path $env:TEMP ('sma-install-{0}.log' -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
function Write-Step([string]$Message) {
    $line = '[{0}] {1}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'),$Message
    Write-Host $line
    Add-Content -LiteralPath $installLog -Value $line -Encoding UTF8
}
function Invoke-Ctl([string[]]$Arguments) {
    & $ctl @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Management command failed: $($Arguments -join ' ')" }
}
try {
    if (-not $MsiPath) {
        $packages = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'sma_*_windows_amd64.msi')
        if ($packages.Count -ne 1) { throw 'Provide -MsiPath or keep exactly one SMA MSI next to this script.' }
        $MsiPath = $packages[0].FullName
    }
    $MsiPath = (Resolve-Path -LiteralPath $MsiPath).Path
    # Verify the shipped MSI when a manifest is present. Obtain the ZIP hash independently.
    $manifest = Join-Path (Split-Path -Parent $MsiPath) 'checksums.txt'
    if (Test-Path -LiteralPath $manifest) {
        $name = Split-Path -Leaf $MsiPath
        $entry = @(Get-Content -LiteralPath $manifest | Where-Object { $_ -match ('^[a-fA-F0-9]{64}\s+\*?' + [regex]::Escape($name) + '$') })
        if ($entry.Count -ne 1) { throw 'MSI checksum entry is missing or ambiguous.' }
        if ((Get-FileHash -LiteralPath $MsiPath -Algorithm SHA256).Hash -ne $entry[0].Substring(0,64)) { throw 'MSI checksum mismatch.' }
    }
    if ($ListenAddress) {
        if ($ListenAddress -notmatch '^(\[[0-9a-fA-F:]+\]|[0-9.]+):([0-9]+)$') { throw 'ListenAddress must be IP:PORT, for example 0.0.0.0:9108.' }
        $listenPort = [int]$Matches[2]
        if ($listenPort -lt 1 -or $listenPort -gt 65535) { throw 'Invalid listen port.' }
    }
    foreach ($source in $AllowedSource) {
        # Firewall accepts IPv4/IPv6 addresses and CIDRs; no Any/Internet shorthand.
        $parts = $source.Split('/')
        $parsed = $null
        if ($parts.Count -gt 2 -or -not [Net.IPAddress]::TryParse($parts[0],[ref]$parsed)) { throw "Invalid AllowedSource: $source" }
        if ($parts.Count -eq 2) {
            $prefix = 0
            $max = if ($parsed.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetwork) {32} else {128}
            if (-not [int]::TryParse($parts[1],[ref]$prefix) -or $prefix -lt 1 -or $prefix -gt $max) { throw "Invalid CIDR: $source" }
        }
    }
    $installed = Get-ItemProperty 'HKLM:\Software\SMA' -ErrorAction SilentlyContinue
    Write-Step ('Current version: ' + $(if ($installed) {$installed.Version} else {'not installed'}))
    Write-Step "Installing package: $(Split-Path -Leaf $MsiPath)"
    $msiLog = "$installLog.msi.log"
    $process = Start-Process msiexec.exe -ArgumentList @('/i',('"{0}"' -f $MsiPath),'/qn','/norestart','/L*v',('"{0}"' -f $msiLog)) -Wait -PassThru
    if ($process.ExitCode -notin @(0,3010)) { throw "MSI failed with code $($process.ExitCode). Details: $msiLog" }
    if ($process.ExitCode -eq 3010) { Write-Step 'Windows requests a reboot; no automatic reboot was performed.' }
    if ($ListenAddress) { Invoke-Ctl -Arguments @('config','listen',$ListenAddress) }
    if ($AllowedSource) {
        $config = Get-Content -Raw -LiteralPath (Join-Path $data 'config.json') | ConvertFrom-Json
        $port = [int]($config.listenAddress -replace '^.*:', '')
        $ruleName = 'SMA-Agent-Inbound'
        if (Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue) {
            Set-NetFirewallRule -Name $ruleName -Enabled True -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -RemoteAddress $AllowedSource -Program (Join-Path $env:ProgramFiles 'SMA/sma.exe') -Profile Any | Out-Null
        } else {
            New-NetFirewallRule -Name $ruleName -DisplayName 'SMA Agent inbound' -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -RemoteAddress $AllowedSource -Program (Join-Path $env:ProgramFiles 'SMA/sma.exe') -Profile Any | Out-Null
        }
        Write-Step "Firewall configured for specified sources, TCP $port."
    }
    Invoke-Ctl -Arguments @('enable')
    Invoke-Ctl -Arguments @('doctor')
    Invoke-Ctl -Arguments @('status')
    Write-Step 'Installation verified. Existing configuration and token retained.'
    Write-Step "Config: $data\config.json"
    Write-Step "Token retrieval: & '$ctl' token show"
    Write-Step "Logs: $data\logs\sma.log"
    Write-Step "Install log: $installLog"
} catch {
    Write-Step ('FAILED: ' + $_.Exception.Message)
    throw
}
# Never put the bearer token into installation logs or MSI command-line properties.
if ($ShowToken) { Invoke-Ctl -Arguments @('token','show') }
