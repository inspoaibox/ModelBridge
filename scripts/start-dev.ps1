[CmdletBinding()]
param(
  [string]$HttpAddr
)

$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$envFile = Join-Path $projectRoot ".env.local"

if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) {
  throw "Missing local environment file: $envFile"
}

Get-Content -LiteralPath $envFile | ForEach-Object {
  $line = $_.Trim()
  if (-not $line -or $line.StartsWith("#")) {
    return
  }

  if ($line -notmatch "^([^=]+)=(.*)$") {
    throw "Invalid environment entry in $envFile"
  }

  $name = $Matches[1].Trim()
  $value = $Matches[2].Trim()
  if ($value.Length -ge 2 -and (
    ($value.StartsWith('"') -and $value.EndsWith('"')) -or
    ($value.StartsWith("'") -and $value.EndsWith("'"))
  )) {
    $value = $value.Substring(1, $value.Length - 2)
  }

  [Environment]::SetEnvironmentVariable($name, $value, "Process")
}

if ($HttpAddr) {
  $env:HTTP_ADDR = $HttpAddr
}

Push-Location $projectRoot
try {
  go run ./cmd/server
} finally {
  Pop-Location
}
