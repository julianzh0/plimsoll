# Installs the plimsoll binary for this Windows machine from a GitHub release and checks its SHA-256 checksum.
#   irm https://raw.githubusercontent.com/julianzh0/plimsoll/main/install.ps1 | iex
# $env:PLIMSOLL_VERSION picks a release (for example v0.4.0; default: the latest).
# $env:PLIMSOLL_INSTALL_DIR picks the folder (default: %LOCALAPPDATA%\plimsoll).
$ErrorActionPreference = 'Stop'

$repo = 'https://github.com/julianzh0/plimsoll'
$version = if ($env:PLIMSOLL_VERSION) { $env:PLIMSOLL_VERSION } else { 'latest' }
$dir = if ($env:PLIMSOLL_INSTALL_DIR) { $env:PLIMSOLL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'plimsoll' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'amd64' }
  'ARM64' { 'arm64' }
  default { throw "plimsoll: no release for $($env:PROCESSOR_ARCHITECTURE); build it with Go instead" }
}
$base = if ($version -eq 'latest') { "$repo/releases/latest/download" } else { "$repo/releases/download/$version" }
$name = "plimsoll_windows_$arch.exe"
$work = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $work | Out-Null
try {
  Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile (Join-Path $work $name)
  Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $work 'checksums.txt')
  $line = Get-Content (Join-Path $work 'checksums.txt') | Where-Object { $_ -match "\s$([regex]::Escape($name))$" }
  if (-not $line) { throw "plimsoll: $name is not in checksums.txt" }
  $want = ($line -split '\s+')[0].ToLower()
  $got = (Get-FileHash -Algorithm SHA256 (Join-Path $work $name)).Hash.ToLower()
  if ($got -ne $want) { throw 'plimsoll: checksum mismatch, nothing installed' }

  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  Move-Item -Force (Join-Path $work $name) (Join-Path $dir 'plimsoll.exe')
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
Write-Host "plimsoll installed to $(Join-Path $dir 'plimsoll.exe')"

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
  [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$dir"), 'User')
  Write-Host "added $dir to your PATH; open a new terminal to run it by name"
}
