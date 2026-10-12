param([Parameter(Mandatory=$true)][string]$SetupPath)
$ErrorActionPreference='Stop'
$fixture=Join-Path $env:TEMP ('thoth-image-selection-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $fixture | Out-Null
Copy-Item $SetupPath (Join-Path $fixture 'setup.ps1')
[IO.File]::WriteAllText((Join-Path $fixture '.env'),"THOTH_IMAGE=example.invalid/selected:fixture`nTHOTH_PORT=17429`n")
Remove-Item Env:THOTH_IMAGE -ErrorAction SilentlyContinue
Remove-Item Env:THOTH_PORT -ErrorAction SilentlyContinue
$global:SelectedImage=''
# Stop at retrieval. This test never invokes Docker or modifies an installation.
function global:docker {
 $global:LASTEXITCODE=0
 if ($args[0] -eq 'pull') { $global:SelectedImage=$args[1];$global:LASTEXITCODE=1 }
}
try { & (Join-Path $fixture 'setup.ps1') } catch {
 if ($_.Exception.Message -notmatch 'Image retrieval failed') { throw }
}
if ($global:SelectedImage -ne 'example.invalid/selected:fixture') { throw "Installer ignored .env image: $global:SelectedImage" }
Write-Host 'Windows existing .env image selection: PASS (Docker stub; no deployment)'
