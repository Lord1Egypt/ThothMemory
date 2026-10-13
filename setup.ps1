param([switch]$Build)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Install Docker Desktop, enable Linux containers, and retry.' }
& docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Start Docker Desktop and retry.' }
& docker compose version
if ($LASTEXITCODE -ne 0) { throw 'Docker Compose is required.' }
# Follow Compose's existing nonsecret settings without executing the file.
if (Test-Path .env) {
    foreach ($line in Get-Content .env) {
        $pair = $line -split '=', 2
        if ($pair.Count -ne 2) { continue }
        if ($pair[0] -eq 'THOTH_IMAGE' -and -not $env:THOTH_IMAGE -and -not $Build) { $env:THOTH_IMAGE = $pair[1].Trim() }
        if ($pair[0] -eq 'THOTH_PORT' -and -not $env:THOTH_PORT) { $env:THOTH_PORT = $pair[1].Trim() }
    }
}
$image = if ($env:THOTH_IMAGE) { $env:THOTH_IMAGE } elseif ($Build) { 'thoth-memory:dev' } else { 'lord1egypt/thoth-memory:0.2.0' }
if ($Build) { $env:THOTH_IMAGE = $image; & docker build -t $image . } else { & docker pull $image }
if ($LASTEXITCODE -ne 0) { throw 'Image retrieval failed.' }
New-Item -ItemType Directory -Force bin | Out-Null
$container = (& docker create $image).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Image extraction failed.' }
try { & docker cp "${container}:/clients/thothmemory.exe" bin/thothmemory.exe; if ($LASTEXITCODE -ne 0) { throw 'CLI extraction failed.' } } finally { & docker rm $container | Out-Null }
& ./bin/thothmemory.exe init-config
if ($LASTEXITCODE -ne 0) { throw 'Configuration generation failed.' }
& ./bin/thothmemory.exe preflight
if ($LASTEXITCODE -ne 0) { throw 'Port preflight failed.' }
$me = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls .secrets /inheritance:r /grant:r "${me}:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Secret directory ACL protection failed.' }
$port = if ($env:THOTH_PORT) { [int]$env:THOTH_PORT } else { 7319 }
$portLine = Get-Content .env | Where-Object { $_ -match '^THOTH_PORT=' }
if ($portLine -and -not $env:THOTH_PORT) { $port = [int]($portLine -split '=',2)[1] }
$occupied = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
if ($occupied) { Write-Host "Port $port is in use; Compose will verify whether this is the existing installation." }
& docker compose up -d --no-build --wait --wait-timeout 120
if ($LASTEXITCODE -ne 0) { throw 'Deployment failed. Run docker compose logs.' }
& ./bin/thothmemory.exe doctor
if ($LASTEXITCODE -ne 0) { throw 'MCP verification failed.' }
Write-Host "Dashboard: http://127.0.0.1:$port; token: .secrets/agent_token"
Write-Host './bin/thothmemory.exe connect all'
Start-Process "http://127.0.0.1:$port"
