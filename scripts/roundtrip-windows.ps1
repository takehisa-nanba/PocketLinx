param(
 [Parameter(Mandatory=$true)][ValidateSet('start','exchange','finish')][string]$Stage,
 [Parameter(Mandatory=$true)][string]$Distro,
 [Parameter(Mandatory=$true)][string]$Root,
 [Parameter(Mandatory=$true)][string]$Artifacts,
 [Parameter(Mandatory=$true)][string]$Sample,
 [string]$InputBundle,
 [string]$WindowsCLI='.\plx.exe',
 [string]$LinuxCLI='/usr/local/bin/plx-env',
 [string]$Check='/usr/local/bin/plx-roundtrip-check',
 [string]$Fixture='/usr/local/bin/plx-wsl-fixture'
)
$ErrorActionPreference='Stop'
function Linux([string[]]$Arguments) {
 $result=& wsl.exe --distribution $Distro --user root --exec @Arguments
 if ($LASTEXITCODE -ne 0) { throw "Linux verifier failed: $Arguments" }
 return $result
}
function Invoke-Adapter([string[]]$Arguments) {
 $result=& $WindowsCLI wsl --distro $Distro --linux-cli $LinuxCLI @Arguments
 if ($LASTEXITCODE -ne 0) { throw "WSL adapter failed: $Arguments" }
 return $result
}
function Write-UTF8([string]$File,[string]$Text) { [IO.File]::WriteAllText($File,$Text,[Text.UTF8Encoding]::new($false)) }
function Execute-Sample([string]$State) {
 $expected=Linux @($Check,'expected',$Root,$LinuxCLI,$State)
 $actual=Invoke-Adapter @('--trusted-sample','run',$Root)
 if (($actual -join "`n") -cne ($expected -join "`n")) { throw 'Definition execution differs' }
 Add-Content -LiteralPath (Join-Path $Artifacts 'output.txt') -Value $actual
}
New-Item -ItemType Directory -Path $Artifacts -ErrorAction Stop | Out-Null
$Artifacts=(Resolve-Path -LiteralPath $Artifacts).Path
Write-UTF8 (Join-Path $Artifacts 'host.txt') ((Linux @($Check,'host','wsl')) -join "`n")
if ($Stage -eq 'start') {
 Linux @($Fixture,'create',$Root,$Sample)
 Linux @($Check,'seed',$Root)
 $state='original'
} else {
 if (!$InputBundle) { throw 'InputBundle required' }
 $InputBundle=(Resolve-Path -LiteralPath $InputBundle).Path
 foreach ($pair in @(@($InputBundle,"$InputBundle.sha256"),@("$InputBundle.files.json","$InputBundle.files.sha256"))) {
  if ((Get-FileHash -LiteralPath $pair[0] -Algorithm SHA256).Hash -ne (Get-Content -Raw -LiteralPath $pair[1]).Trim()) { throw 'Transfer SHA-256 mismatch' }
 }
 Invoke-Adapter @('restore',$InputBundle,$Root)
 & $WindowsCLI wsl --distro $Distro --linux-cli $LinuxCLI restore $InputBundle $Root
 if ($LASTEXITCODE -eq 0) { throw 'Environment overwrite accepted' }
 # Only JSON evidence goes through the text pipe; packages use wslbridge binary I/O.
 [IO.File]::ReadAllText("$InputBundle.files.json") | & wsl.exe --distribution $Distro --user root --exec $Check check $Root -
 if ($LASTEXITCODE -ne 0) { throw 'Restored attributes differ from sender evidence' }
 $state=(Get-Content -Raw -LiteralPath "$InputBundle.state").Trim()
 if (($Stage -eq 'exchange' -and $state -ne 'original') -or ($Stage -eq 'finish' -and $state -ne 'returned')) { throw 'Incorrect exchange stage' }
}
Execute-Sample $state
Write-UTF8 (Join-Path $Artifacts 'permission-error.txt') ((Linux @($Check,'denied',$Root,$LinuxCLI)) -join "`n")
if ($Stage -eq 'exchange') {
 Linux @($Fixture,'edit',$Root,$Sample)
 Linux @($Check,'delete',$Root)
 Execute-Sample 'updated'
 $state='returned'
}
if ($Stage -ne 'finish') {
 $output=Join-Path $Artifacts 'package.plxenv'
 Write-UTF8 "$output.files.json" ((Linux @($Check,'snapshot',$Root)) -join "`n")
 Invoke-Adapter @('--stopped','save',$Root,$output)
 Write-UTF8 "$output.sha256" ((Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash.ToLowerInvariant())
 Write-UTF8 "$output.files.sha256" ((Get-FileHash -LiteralPath "$output.files.json" -Algorithm SHA256).Hash.ToLowerInvariant())
 Write-UTF8 "$output.state" $state
 & $WindowsCLI wsl --distro $Distro --linux-cli $LinuxCLI --stopped save $Root $output
 if ($LASTEXITCODE -eq 0) { throw 'Overwrite accepted' }
 if ((Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash -ne (Get-Content -Raw -LiteralPath "$output.sha256")) { throw 'Existing package changed' }
}
Write-UTF8 (Join-Path $Artifacts 'result.txt') "PASS stage=$Stage role=wsl (independent Linux round trip requires the other host)"
Get-Content -LiteralPath (Join-Path $Artifacts 'result.txt')
$global:LASTEXITCODE=0
