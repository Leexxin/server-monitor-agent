[CmdletBinding()]
param([string]$Version = '0.3.0-rc.1', [string]$MsiVersion = '0.3.0', [string]$Revision = 'unknown')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$project = Split-Path -Parent $PSScriptRoot
Push-Location $project
try {
    if ($Version -notmatch '^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$') { throw 'Invalid version' }
    $stage = Join-Path $project "dist/windows-$Version"
    if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
    $payload = Join-Path $stage 'payload'
    New-Item -ItemType Directory -Force $payload | Out-Null
    $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
    $built = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $flags = "-s -w -X sma/internal/buildinfo.Version=v$Version -X sma/internal/buildinfo.Revision=$Revision -X sma/internal/buildinfo.BuildTime=$built"
    foreach ($name in @('sma','sma-ctl')) {
        & go build -trimpath -ldflags $flags -o (Join-Path $payload "$name.exe") "./cmd/$name"
        if ($LASTEXITCODE -ne 0) { throw "Build failed: $name" }
    }
    Copy-Item -Recurse -Force docs $payload
    Copy-Item README.md,CHANGELOG.md $payload
    Copy-Item packaging/windows/*-sma.ps1,packaging/windows/test-windows.ps1 $payload
    $wixBin = if ($env:WIX) { Join-Path $env:WIX 'bin' } else { '' }
    if (-not (Test-Path (Join-Path $wixBin 'candle.exe'))) {
        $found = Get-ChildItem "${env:ProgramFiles(x86)}\WiX Toolset*\bin\candle.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $found) { throw 'WiX Toolset 3.14 is required on the Windows build machine.' }
        $wixBin = $found.DirectoryName
    }
    # Harvest only support files. The executable/service components have stable IDs above.
    $support = Join-Path $stage 'support'
    New-Item -ItemType Directory -Force $support | Out-Null
    Get-ChildItem $payload | Where-Object Extension -ne '.exe' | Copy-Item -Destination $support -Recurse -Force
    & "$wixBin/heat.exe" dir $support -nologo -ag -srd -sfrag -dr INSTALLDIR -cg SupportFiles -var var.Support -out "$stage/support.wxs"
    if ($LASTEXITCODE -ne 0) { throw 'WiX harvest failed' }
    & "$wixBin/candle.exe" -nologo -arch x64 -ext WixUtilExtension "-dPayload=$payload" "-dSupport=$support" "-dMsiVersion=$MsiVersion" "-dDisplayVersion=$Version" -out "$stage/" packaging/windows/sma.wxs "$stage/support.wxs"
    if ($LASTEXITCODE -ne 0) { throw 'WiX compile failed' }
    $out = Join-Path $project 'dist/windows'
    New-Item -ItemType Directory -Force $out | Out-Null
    $msi = Join-Path $out "sma_${Version}_windows_amd64.msi"
    & "$wixBin/light.exe" -nologo -ext WixUtilExtension -out $msi "$stage/sma.wixobj" "$stage/support.wixobj"
    if ($LASTEXITCODE -ne 0) { throw 'MSI link/validation failed' }
    Copy-Item $msi $payload -Force
    $manifest = Get-ChildItem $payload -Recurse -File | ForEach-Object {
        $relative = $_.FullName.Substring($payload.Length+1).Replace('\','/')
        '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$relative
    }
    $manifest | Set-Content (Join-Path $payload 'checksums.txt') -Encoding ASCII
    Compress-Archive -Path "$payload/*" -DestinationPath "$out/sma_${Version}_windows_amd64.zip" -Force
    Get-ChildItem $out -File | Where-Object Extension -in '.zip','.msi' | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$_.Name
    } | Set-Content "$out/checksums-windows.txt" -Encoding ASCII
    Write-Host "Windows packages created: $out"
} finally { Pop-Location }
