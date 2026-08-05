param([string]$InstallDir = "")
$ErrorActionPreference = "Stop"
$Name = "diss"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$CommandPath = (Get-Command $Name -ErrorAction SilentlyContinue).Source

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    if ($CommandPath -and (Test-Path $CommandPath)) {
        $InstallDir = Split-Path -Parent $CommandPath
    } else {
        $InstallDir = Join-Path $env:USERPROFILE ".local\bin"
    }
}

$BuildArtifact = Join-Path $Root ".$Name.build.$PID.exe"
$Target = Join-Path $InstallDir "$Name.exe"
$TargetArtifact = Join-Path $InstallDir ".$Name.install.$PID.exe"

try {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

    if (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host "Building $Name from source with $((Get-Command go).Source)"
        Push-Location $Root
        try { go build -o $BuildArtifact . } finally { Pop-Location }
        $Artifact = $BuildArtifact
    } else {
        $Artifact = Join-Path $Root "releases\$Name-windows-amd64.exe"
        if (-not (Test-Path $Artifact)) { throw "Go is unavailable and no matching release artifact exists: $Artifact" }
        Write-Host "Go unavailable; installing release artifact $Artifact"
    }

    Copy-Item -Force $Artifact $TargetArtifact
    Move-Item -Force $TargetArtifact $Target
    if (-not (Test-Path $Target)) { throw "Installation failed; executable not found: $Target" }
    Write-Host "Installed $Name to $Target"
} finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $BuildArtifact, $TargetArtifact
}

$Resolved = (Get-Command $Name -ErrorAction SilentlyContinue).Source
if (-not $Resolved) {
    Write-Warning "$Target is not currently in PATH; add $InstallDir to PATH."
} elseif ((Resolve-Path $Resolved).Path -ne (Resolve-Path $Target).Path) {
    Write-Warning "PATH resolves $Resolved, not the newly installed $Target."
}
