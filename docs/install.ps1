$ErrorActionPreference = "Stop"

$repo = "kunchenguid/treehouse"
$installDir = "$env:LOCALAPPDATA\treehouse"

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name
$versionNum = $version.TrimStart("v")

$filename = "treehouse-v$versionNum-windows-$arch.zip"
$url = "https://github.com/$repo/releases/download/$version/$filename"

$tmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }

Write-Host "Downloading treehouse $version for windows/$arch..."
Invoke-WebRequest -Uri $url -OutFile "$tmpDir\$filename"

# Verify the SHA-256 checksum against the published checksums.txt before
# extracting, matching the integrity check the in-app updater performs.
Write-Host "Verifying checksum..."
$checksumsFile = "$tmpDir\checksums.txt"
Invoke-WebRequest -Uri "https://github.com/$repo/releases/download/$version/checksums.txt" -OutFile $checksumsFile

$expected = $null
foreach ($line in Get-Content $checksumsFile) {
    $fields = $line -split '\s+', 2
    if ($fields.Count -eq 2 -and $fields[1].Trim() -eq $filename) {
        $expected = $fields[0].Trim().ToLower()
        break
    }
}
if (-not $expected) {
    throw "No checksum found for $filename in checksums.txt"
}

$actual = (Get-FileHash -Algorithm SHA256 -Path "$tmpDir\$filename").Hash.ToLower()
if ($actual -ne $expected) {
    throw "Checksum verification failed for $filename`n  expected: $expected`n  actual:   $actual"
}

Expand-Archive -Path "$tmpDir\$filename" -DestinationPath $tmpDir -Force

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
Move-Item -Path "$tmpDir\treehouse.exe" -Destination "$installDir\treehouse.exe" -Force

Remove-Item -Recurse -Force $tmpDir

# Add to PATH if not already there
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "Added $installDir to user PATH. Restart your terminal for it to take effect."
}

Write-Host "treehouse $version installed to $installDir\treehouse.exe"
