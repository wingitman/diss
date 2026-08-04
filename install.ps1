param([string]$InstallDir = "$env:USERPROFILE\.local\bin")
$ErrorActionPreference = "Stop"
$Name = "diss"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Artifact = $null

if (Get-Command go -ErrorAction SilentlyContinue) {
    Push-Location $Root
    try { go build -o "$Name.exe" .; $Artifact = Join-Path $Root "$Name.exe" } finally { Pop-Location }
} else {
    $Artifact = Join-Path $Root "releases\$Name-windows-amd64.exe"
    if (-not (Test-Path $Artifact)) { throw "Go is unavailable and no matching release artifact exists." }
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Force $Artifact (Join-Path $InstallDir "$Name.exe")
Write-Host "Installed $Name to $InstallDir"
