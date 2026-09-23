param(
    [Parameter(Mandatory=$true)][string]$Distro,
    [Parameter(Mandatory=$true)][string]$WindowsCLI,
    [Parameter(Mandatory=$true)][string]$LinuxSample,
    [string]$LinuxCLI = '/usr/local/bin/plx-env',
    [string]$LinuxFixture = '/usr/local/bin/plx-wsl-fixture',
    [string]$OutputDirectory = (Join-Path $env:TEMP ('plx-wsl-' + [guid]::NewGuid().ToString('N')))
)
$ErrorActionPreference = 'Stop'
function Invoke-CLI([string[]]$Arguments) {
    $result = & $WindowsCLI wsl --distro $Distro --linux-cli $LinuxCLI @Arguments
    if ($LASTEXITCODE -ne 0) { throw "CLI failed ($LASTEXITCODE): $Arguments" }
    return $result
}
function Invoke-Linux([string[]]$Arguments) {
    $result = & wsl.exe --distribution $Distro --user root --exec @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Linux fixture failed ($LASTEXITCODE): $Arguments" }
    return $result
}
function Expect-Failure([string[]]$Arguments) {
    & $WindowsCLI @Arguments
    if ($LASTEXITCODE -eq 0) { throw "Expected failure: $Arguments" }
}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$watch = [Diagnostics.Stopwatch]::StartNew()
$base = '/opt/pocketlinx/verify-' + [guid]::NewGuid().ToString('N')
Invoke-Linux @('/bin/mkdir', $base)
$original = "$base/original"
$restored = "$base/restored space ' quote"
$back = "$base/back"
$first = Join-Path $OutputDirectory "first space ' quote.plxenv"
$second = Join-Path $OutputDirectory 'second.plxenv'
Invoke-CLI @('check')
Invoke-Linux @($LinuxFixture, 'create', $original, $LinuxSample)
$definition = (Invoke-Linux @('/bin/cat', "$original/environment.json")) | ConvertFrom-Json
$expected = '{0}|{1}|persistent-data|uid={2}|gid={3}' -f $definition.env.GREETING, $definition.workdir, $definition.uid, $definition.gid
$before = Invoke-CLI @('--trusted-sample', 'run', $original)
if ($before -ne $expected) { throw 'Definition not reproduced before save' }
Invoke-CLI @('--stopped', 'save', $original, $first)
Invoke-CLI @('restore', $first, $restored)
$after = Invoke-CLI @('--trusted-sample', 'run', $restored)
if ($after -ne $before) { throw 'Restored execution differs' }
Invoke-Linux @($LinuxFixture, 'edit', $restored, $LinuxSample)
$changed = Invoke-CLI @('--trusted-sample', 'run', $restored)
if (($changed -join "`n") -ne ($expected.Replace('persistent-data','updated-data') + "`nedited")) { throw 'Changed execution differs' }
Invoke-CLI @('--stopped', 'save', $restored, $second)
Invoke-CLI @('restore', $second, $back)
$roundTrip = Invoke-CLI @('--trusted-sample', 'run', $back)
if (($roundTrip -join "`n") -ne ($expected.Replace('persistent-data','runtime-write') + "`nedited")) { throw 'Resaved execution differs' }
$generated = Invoke-Linux @('/bin/cat', "$back/source/generated.txt")
if ($generated -ne 'source-write') { throw 'Source write not preserved' }
$prefix = @('wsl', '--distro', $Distro, '--linux-cli', $LinuxCLI)
$originalHash = (Get-FileHash -LiteralPath $first -Algorithm SHA256).Hash
Expect-Failure ($prefix + @('--stopped', 'save', $restored, $first))
if ((Get-FileHash -LiteralPath $first -Algorithm SHA256).Hash -ne $originalHash) { throw 'Existing package changed' }
Expect-Failure ($prefix + @('restore', $first, $restored))
Expect-Failure ($prefix + @('--user', 'nobody', '--trusted-sample', 'run', $restored))
Expect-Failure ($prefix + @('--user', 'nobody', 'restore', $first, "$base/denied"))
Expect-Failure ($prefix + @('--stopped', 'save', $original, (Join-Path $OutputDirectory 'missing-parent/out.plxenv')))
Expect-Failure ($prefix + @('restore', $first, '/mnt/c/plx-must-not-restore'))
Expect-Failure ($prefix + @('restore', $first, "$base/missing-parent/env"))
Expect-Failure @('wsl', '--distro', 'PocketLinx-Nonexistent-Test', 'check')
$afterErrors = Invoke-CLI @('--trusted-sample', 'run', $original)
if ($afterErrors -ne $before) { throw 'Original changed after error cases' }
Invoke-Linux @($LinuxFixture, 'exit', $back, $LinuxSample)
& $WindowsCLI @prefix --trusted-sample run $back
if ($LASTEXITCODE -ne 37) { throw 'Linux exit code 37 was not propagated to Windows' }
$watch.Stop()
[pscustomobject]@{
    Result='PASS'; Distro=$Distro; LinuxArtifacts=$base; WindowsArtifacts=$OutputDirectory
    FirstSHA256=$originalHash; SecondSHA256=(Get-FileHash -LiteralPath $second -Algorithm SHA256).Hash
    ElapsedSeconds=$watch.Elapsed.TotalSeconds
} | Format-List
# Artifacts are deliberately retained for inspection. No distro is reset/deleted.
