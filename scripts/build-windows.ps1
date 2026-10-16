[CmdletBinding()]
param([string]$Version = '0.3.1-rc.1', [string]$MsiVersion = '0.3.1', [string]$Revision = 'unknown')
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
    $csc = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    $guiSources = @(Get-ChildItem (Join-Path $project 'windows/gui') -Filter '*.cs' | ForEach-Object FullName)
    & $csc /nologo /utf8output /codepage:65001 /target:winexe /platform:x64 /optimize+ /win32manifest:windows/gui/app.manifest "/out:$payload/sma-gui.exe" /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.ServiceProcess.dll /reference:System.Web.Extensions.dll @guiSources
    if ($LASTEXITCODE -ne 0) { throw 'GUI compilation failed' }
    Copy-Item windows/gui/app.config "$payload/sma-gui.exe.config"
    Copy-Item -Recurse -Force docs $payload
    Copy-Item README.md,CHANGELOG.md $payload
    Copy-Item packaging/windows/*-sma.ps1,packaging/windows/test-windows.ps1 $payload
    Copy-Item packaging/windows/configure-network.ps1 $payload
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
    & "$wixBin/candle.exe" -nologo -arch x64 -ext WixUtilExtension -ext WixUIExtension "-dPayload=$payload" "-dSupport=$support" "-dMsiVersion=$MsiVersion" "-dDisplayVersion=$Version" -out "$stage/" packaging/windows/sma.wxs packaging/windows/ui.wxs "$stage/support.wxs"
    if ($LASTEXITCODE -ne 0) { throw 'WiX compile failed' }
    $out = Join-Path $project 'dist/windows'
    New-Item -ItemType Directory -Force $out | Out-Null
    $msi = Join-Path $out "sma_${Version}_windows_amd64.msi"
    & "$wixBin/light.exe" -nologo -ext WixUtilExtension -ext WixUIExtension -cultures:zh-cn -out $msi "$stage/sma.wixobj" "$stage/ui.wixobj" "$stage/support.wixobj"
    if ($LASTEXITCODE -ne 0) { throw 'MSI link/validation failed' }
    Copy-Item $msi $payload -Force
    $manifest = Get-ChildItem $payload -Recurse -File | ForEach-Object {
        $relative = $_.FullName.Substring($payload.Length+1).Replace('\','/')
        '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$relative
    }
    [IO.File]::WriteAllText((Join-Path $payload 'checksums.txt'),($manifest -join "`n")+"`n",[Text.Encoding]::ASCII)
    Add-Type -AssemblyName System.IO.Compression,System.IO.Compression.FileSystem
    $zip = "$out/sma_${Version}_windows_amd64.zip"
    if (Test-Path -LiteralPath $zip) { Remove-Item -LiteralPath $zip -Force }
    $archive = [IO.Compression.ZipFile]::Open($zip,[IO.Compression.ZipArchiveMode]::Create)
    try {
        Get-ChildItem $payload -Recurse -File | ForEach-Object {
            $relative = $_.FullName.Substring($payload.Length+1).Replace('\','/')
            [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive,$_.FullName,$relative,[IO.Compression.CompressionLevel]::Optimal)
        }
    } finally { $archive.Dispose() }
    $checksums = Get-ChildItem $out -File | Where-Object Extension -in '.zip','.msi' | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$_.Name
    }
    [IO.File]::WriteAllText("$out/checksums-windows.txt",($checksums -join "`n")+"`n",[Text.Encoding]::ASCII)
    Write-Host "Windows packages created: $out"
} finally { Pop-Location }
