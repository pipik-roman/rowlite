$ErrorActionPreference = 'Stop'
$Repo = "pipik-roman/rowlite"
$BinName = "rowlite.exe"
$InstallDir = "$env:LOCALAPPDATA\Programs\rowlite"

# Detect Architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
        "arm64"
    } else {
        "amd64"
    }
} else {
    Write-Error "Unsupported architecture."
    exit 1
}

Write-Host "Fetching latest release for $Repo..." -ForegroundColor Cyan
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
$Tag = $Release.tag_name
$Version = $Tag.TrimStart('v')

$AssetUrl = "https://github.com/$Repo/releases/download/$Tag/rowlite_${Version}_windows_${Arch}.zip"
$TempZip = [System.IO.Path]::GetTempFileName() + ".zip"
$TempExtract = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())

try {
    Write-Host "Downloading Rowlite $Tag ($Arch)..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $AssetUrl -OutFile $TempZip

    Write-Host "Extracting binary..." -ForegroundColor Cyan
    Expand-Archive -Path $TempZip -DestinationPath $TempExtract -Force

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    Copy-Item -Path "$TempExtract\$BinName" -Destination "$InstallDir\$BinName" -Force

    # Automatically add to User PATH if missing
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        Write-Host "Adding $InstallDir to your User PATH..." -ForegroundColor Yellow
        [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
        $env:Path += ";$InstallDir"
    }

    Write-Host "`nRowlite $Tag installed successfully!" -ForegroundColor Green
    Write-Host "Location: $InstallDir\$BinName" -ForegroundColor Gray
    Write-Host "Note: Restart your terminal if 'rowlite' is not recognized immediately." -ForegroundColor Yellow
} finally {
    if (Test-Path $TempZip) { Remove-Item $TempZip -Force -ErrorAction SilentlyContinue }
    if (Test-Path $TempExtract) { Remove-Item $TempExtract -Recurse -Force -ErrorAction SilentlyContinue }
}
