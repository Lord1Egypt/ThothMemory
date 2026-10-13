param([switch]$Build)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Install Docker Desktop, enable Linux containers, and retry.' }
& docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Start Docker Desktop and retry.' }
& docker compose version
if ($LASTEXITCODE -ne 0) { throw 'Docker Compose is required.' }
$image = if ($env:THOTH_IMAGE) { $env:THOTH_IMAGE } else { 'lord1egypt/thoth-memory:0.1.2' }
if ($Build) { $image = 'thoth-memory:dev'; $env:THOTH_IMAGE = $image; & docker build -t $image . } else { & docker pull $image }
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
$port = 7319
$portLine = Get-Content .env | Where-Object { $_ -match '^THOTH_PORT=' }
if ($portLine) { $port = [int]($portLine -split '=',2)[1] }
$occupied = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
if ($occupied) { Write-Host "Port $port is in use; Compose will verify whether this is the existing installation." }
& docker compose up -d --no-build --wait --wait-timeout 120
if ($LASTEXITCODE -ne 0) { throw 'Deployment failed. Run docker compose logs.' }
& ./bin/thothmemory.exe doctor
if ($LASTEXITCODE -ne 0) { throw 'MCP verification failed.' }
Write-Host "Dashboard: http://127.0.0.1:$port; token: .secrets/agent_token"
Write-Host './bin/thothmemory.exe connect all'
Start-Process "http://127.0.0.1:$port"
