# install.ps1 - Install azdo-tui from GitHub releases (Windows PowerShell)
#
# Usage:
#   irm https://raw.githubusercontent.com/Elpulgo/azdo/main/install.ps1 | iex
#   .\install.ps1
#   .\install.ps1 -Version v0.1.0
#   .\install.ps1 -InstallDir "C:\custom\path"

[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$InstallDir = "",
    [switch]$SkipConfig,
    [switch]$Help
)

# ─── Configuration ─────────────────────────────────────────────────────────────

$RepoOwner = "Elpulgo"
$RepoName = "azdo"
$BinaryName = "azdo"
$ConfigDirName = "azdo-tui"
$GitHubApi = "https://api.github.com"
$GitHubDownload = "https://github.com"

$ErrorActionPreference = "Stop"

# ─── Output helpers ────────────────────────────────────────────────────────────

function Write-Step($msg) {
    Write-Host ""
    Write-Host "==> $msg" -ForegroundColor Cyan
}

function Write-Info($msg) {
    Write-Host "[INFO]  $msg" -ForegroundColor Blue
}

function Write-Ok($msg) {
    Write-Host "[OK]    $msg" -ForegroundColor Green
}

function Write-Warn($msg) {
    Write-Host "[WARN]  $msg" -ForegroundColor Yellow
}

function Write-Err($msg) {
    Write-Host "[ERROR] $msg" -ForegroundColor Red
}

# ─── Help ──────────────────────────────────────────────────────────────────────

function Show-Usage {
    Write-Host @"

Usage: .\install.ps1 [OPTIONS]

Install azdo-tui from GitHub releases.

Options:
  -Version VERSION     Install a specific version (e.g., v0.1.0)
                       Default: latest release
  -InstallDir DIR      Custom installation directory
                       Default: %LOCALAPPDATA%\azdo-tui
  -SkipConfig          Skip creating the configuration file
  -Help                Show this help message

Examples:
  # Install latest version
  irm https://raw.githubusercontent.com/Elpulgo/azdo/main/install.ps1 | iex

  # Install specific version
  .\install.ps1 -Version v0.1.0

  # Install to custom directory
  .\install.ps1 -InstallDir "C:\tools\azdo-tui"
"@
}

if ($Help) {
    Show-Usage
    exit 0
}

# ─── Architecture detection ───────────────────────────────────────────────────

function Get-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64"   { return "x86_64" }
        "Arm64" { return "arm64" }
        default {
            # Fallback for older PowerShell
            $envArch = $env:PROCESSOR_ARCHITECTURE
            switch ($envArch) {
                "AMD64" { return "x86_64" }
                "ARM64" { return "arm64" }
                default {
                    Write-Err "Unsupported architecture: $envArch"
                    Write-Err "azdo-tui supports x86_64 (amd64) and arm64."
                    exit 1
                }
            }
        }
    }
}

# ─── Version resolution ───────────────────────────────────────────────────────

function Get-LatestVersion {
    if ($Version) {
        if (-not $Version.StartsWith("v")) {
            $script:Version = "v$Version"
        }
        Write-Ok "Using specified version: $Version"
        return $Version
    }

    Write-Info "Fetching latest release version..."

    try {
        $releaseUrl = "$GitHubApi/repos/$RepoOwner/$RepoName/releases/latest"
        $response = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "azdo-tui-installer" }
        $script:Version = $response.tag_name

        if (-not $Version) {
            Write-Err "Could not determine the latest version."
            Write-Err ""
            Write-Err "You can specify a version manually: .\install.ps1 -Version v0.1.0"
            Write-Err "Check available releases at: $GitHubDownload/$RepoOwner/$RepoName/releases"
            exit 1
        }

        Write-Ok "Latest version: $Version"
        return $Version
    }
    catch {
        Write-Err "Failed to fetch latest release from GitHub."
        Write-Err ""
        Write-Err "Possible causes:"
        Write-Err "  - No releases have been published yet"
        Write-Err "  - GitHub API rate limit exceeded (try again later)"
        Write-Err "  - Network connectivity issues"
        Write-Err ""
        Write-Err "You can specify a version manually: .\install.ps1 -Version v0.1.0"
        Write-Err "Check available releases at: $GitHubDownload/$RepoOwner/$RepoName/releases"
        exit 1
    }
}

# ─── Install directory resolution ─────────────────────────────────────────────

function Get-InstallDirectory {
    if ($InstallDir) {
        Write-Ok "Using custom install directory: $InstallDir"
        return $InstallDir
    }

    $defaultDir = Join-Path $env:LOCALAPPDATA $BinaryName
    Write-Ok "Install directory: $defaultDir"
    return $defaultDir
}

# ─── Download and install ─────────────────────────────────────────────────────

function Install-Binary {
    param(
        [string]$ArchName,
        [string]$TargetDir
    )

    $versionNum = $Version.TrimStart("v")
    $archiveName = "${BinaryName}_${versionNum}_Windows_${ArchName}.zip"
    $downloadUrl = "$GitHubDownload/$RepoOwner/$RepoName/releases/download/$Version/$archiveName"
    $checksumsUrl = "$GitHubDownload/$RepoOwner/$RepoName/releases/download/$Version/checksums.txt"

    Write-Info "Downloading $archiveName..."

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "azdo-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        $archivePath = Join-Path $tempDir $archiveName

        try {
            Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
        }
        catch {
            Write-Err "Failed to download $archiveName"
            Write-Err ""
            Write-Err "Possible causes:"
            Write-Err "  - Version $Version may not exist"
            Write-Err "  - The release may not include a build for Windows/$ArchName"
            Write-Err "  - Network connectivity issues"
            Write-Err ""
            Write-Err "Check available releases at:"
            Write-Err "  $GitHubDownload/$RepoOwner/$RepoName/releases"
            exit 1
        }

        Write-Ok "Downloaded $archiveName"

        # Verify checksum
        Write-Info "Verifying checksum..."
        try {
            $checksumsPath = Join-Path $tempDir "checksums.txt"
            Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

            $checksums = Get-Content $checksumsPath
            $expectedLine = $checksums | Where-Object { $_ -match $archiveName }

            if ($expectedLine) {
                $expectedHash = ($expectedLine -split "\s+")[0]
                $actualHash = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()

                if ($expectedHash -eq $actualHash) {
                    Write-Ok "Checksum verified"
                }
                else {
                    Write-Err "Checksum verification failed!"
                    Write-Err "  Expected: $expectedHash"
                    Write-Err "  Got:      $actualHash"
                    Write-Err "The downloaded file may be corrupted. Please try again."
                    exit 1
                }
            }
            else {
                Write-Warn "Archive not found in checksums file. Skipping verification."
            }
        }
        catch {
            Write-Warn "Could not download checksums file. Skipping verification."
        }

        # Extract archive
        Write-Info "Extracting archive..."
        $extractDir = Join-Path $tempDir "extracted"
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force
        Write-Ok "Extracted archive"

        # Find the binary
        $binaryFile = Get-ChildItem -Path $extractDir -Recurse -Filter "${BinaryName}.exe" | Select-Object -First 1

        if (-not $binaryFile) {
            Write-Err "Could not find '${BinaryName}.exe' in the extracted archive."
            Write-Err "Please report this issue at: https://github.com/$RepoOwner/$RepoName/issues"
            exit 1
        }

        # Create install directory
        if (-not (Test-Path $TargetDir)) {
            Write-Info "Creating install directory: $TargetDir"
            try {
                New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
            }
            catch {
                Write-Err "Failed to create directory: $TargetDir"
                Write-Err ""
                Write-Err "Possible fixes:"
                Write-Err "  - Run PowerShell as Administrator"
                Write-Err "  - Use a custom directory: .\install.ps1 -InstallDir `"C:\Users\$env:USERNAME\bin`""
                exit 1
            }
        }

        # Copy binary
        Write-Info "Installing ${BinaryName}.exe to $TargetDir..."
        Copy-Item -Path $binaryFile.FullName -Destination (Join-Path $TargetDir "${BinaryName}.exe") -Force
        Write-Ok "Installed ${BinaryName}.exe to $TargetDir"

        # Add to PATH if not already there
        $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($currentPath -notlike "*$TargetDir*") {
            Write-Info "Adding $TargetDir to user PATH..."
            try {
                [Environment]::SetEnvironmentVariable(
                    "PATH",
                    "$currentPath;$TargetDir",
                    "User"
                )
                $env:PATH = "$env:PATH;$TargetDir"
                Write-Ok "Added to PATH (restart your terminal for changes to take effect)"
            }
            catch {
                Write-Warn "Could not update PATH automatically."
                Write-Warn "Add this directory to your PATH manually: $TargetDir"
            }
        }
        else {
            Write-Ok "$TargetDir is already in PATH"
        }
    }
    finally {
        # Cleanup
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ─── Configuration file setup ─────────────────────────────────────────────────

function Setup-Config {
    if ($SkipConfig) {
        Write-Info "Skipping configuration file setup (-SkipConfig)"
        return
    }

    $configDir = Join-Path $env:USERPROFILE ".config\$ConfigDirName"
    $script:ConfigFile = Join-Path $configDir "config.yaml"

    if (Test-Path $ConfigFile) {
        Write-Ok "Configuration file already exists at $ConfigFile"
        Write-Info "Skipping config creation to preserve your settings."
        return
    }

    Write-Info "Creating configuration directory: $configDir"
    try {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    }
    catch {
        Write-Err "Failed to create config directory: $configDir"
        Write-Err "You can create it manually later."
        return
    }

    Write-Info "Creating configuration file with placeholder values..."

    $configContent = @"
# Azure DevOps TUI Configuration
# Documentation: https://github.com/$RepoOwner/$RepoName#configuration

# Azure DevOps organization name (required)
# Replace with your organization name from https://dev.azure.com/{organization}
organization: your-org-name

# Azure DevOps project name(s) (required)
# Simple format:
projects:
  - your-project-name

# With display names (friendly name shown in UI):
#   projects:
#     - name: ugly-api-project-name
#       display_name: My Project
#     - simple-project

# Polling interval in seconds (optional, default: 60)
polling_interval: 60

# Theme (optional, default: dark)
# Available: dark, gruvbox, nord, dracula, catppuccin, github, retro
# Custom themes: Place JSON files in ~/.config/azdo-tui/themes/
theme: dark
"@

    Set-Content -Path $ConfigFile -Value $configContent -Encoding UTF8
    Write-Ok "Created configuration file at $ConfigFile"
    Write-Warn "Edit this file with your Azure DevOps organization and project details before running azdo-tui."
}

# ─── Summary ───────────────────────────────────────────────────────────────────

function Show-Summary {
    Write-Host ""
    Write-Host "────────────────────────────────────────────────" -ForegroundColor Green
    Write-Host "  azdo-tui $Version installed successfully!" -ForegroundColor Green
    Write-Host "────────────────────────────────────────────────" -ForegroundColor Green
    Write-Host ""

    if (-not $SkipConfig -and $ConfigFile -and (Test-Path $ConfigFile)) {
        Write-Host "  Next steps:"
        Write-Host "    1. Edit your config file:"
        Write-Host "       $ConfigFile" -ForegroundColor Cyan
        Write-Host "    2. Set your organization and project name(s)"
        Write-Host "    3. Run " -NoNewline
        Write-Host "azdo-tui" -ForegroundColor Cyan
        Write-Host "       (You'll be prompted for your Azure DevOps PAT on first run)"
    }
    else {
        Write-Host "  Next steps:"
        Write-Host "    1. Create a config file at " -NoNewline
        Write-Host "$env:USERPROFILE\.config\$ConfigDirName\config.yaml" -ForegroundColor Cyan
        Write-Host "    2. Run " -NoNewline
        Write-Host "azdo-tui" -ForegroundColor Cyan
    }

    Write-Host ""
    Write-Host "  Documentation: https://github.com/$RepoOwner/$RepoName#readme"
    Write-Host ""
}

# ─── Main ──────────────────────────────────────────────────────────────────────

function Main {
    Write-Host ""
    Write-Host "azdo-tui installer" -ForegroundColor White
    Write-Host "Azure DevOps TUI for your terminal"
    Write-Host ""

    Write-Step "Detecting system"
    Write-Ok "Detected OS: Windows"
    $archName = Get-Architecture
    Write-Ok "Detected architecture: $archName"

    Write-Step "Resolving version"
    Get-LatestVersion | Out-Null
    $installDir = Get-InstallDirectory

    Write-Step "Downloading and installing"
    Install-Binary -ArchName $archName -TargetDir $installDir

    Write-Step "Setting up configuration"
    Setup-Config

    Show-Summary
}

Main
