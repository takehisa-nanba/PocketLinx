# Windows/WSL warm baseline only. Never stops distros, Docker, or existing workloads.
param(
 [Parameter(Mandatory=$true)][string]$Artifacts,
 [Parameter(Mandatory=$true)][string]$LinuxArtifacts,
 [string]$Distro='PocketLinx-Verify',
 [string]$WindowsCLI='.\plx.exe',
 [string]$LinuxCLI='/usr/local/bin/plx-env',
 [string]$Fixture='/usr/local/bin/plx-wsl-fixture',
 [Parameter(Mandatory=$true)][string]$Sample,
 [ValidateRange(3,20)][int]$Repetitions=5
)
$ErrorActionPreference='Stop'
$WindowsCLI=(Resolve-Path -LiteralPath $WindowsCLI).Path
New-Item -ItemType Directory -Path $Artifacts -ErrorAction Stop | Out-Null
$Artifacts=(Resolve-Path -LiteralPath $Artifacts).Path
$id='plx-measure-'+[guid]::NewGuid().ToString('N').Substring(0,12)
$linuxRoot="/opt/pocketlinx/$id"
$script:rows=[Collections.Generic.List[object]]::new()
$script:created=[Collections.Generic.List[string]]::new()
function Native([string]$Exe,[string[]]$Arguments) {
 $psi=[Diagnostics.ProcessStartInfo]::new()
 $psi.FileName=$Exe; $psi.UseShellExecute=$false
 $psi.RedirectStandardOutput=$true; $psi.RedirectStandardError=$true
 if($Exe -eq 'wsl.exe' -and $Arguments[0] -in '--version','--list'){$psi.StandardOutputEncoding=[Text.Encoding]::Unicode}
 foreach($arg in $Arguments){[void]$psi.ArgumentList.Add($arg)}
 $p=[Diagnostics.Process]::new();$p.StartInfo=$psi
 $watch=[Diagnostics.Stopwatch]::StartNew()
 [void]$p.Start();$stdout=$p.StandardOutput.ReadToEndAsync();$stderr=$p.StandardError.ReadToEndAsync()
 $p.WaitForExit();$watch.Stop()
 $result=[pscustomobject]@{Milliseconds=$watch.Elapsed.TotalMilliseconds;Stdout=$stdout.GetAwaiter().GetResult();Stderr=$stderr.GetAwaiter().GetResult();ExitCode=$p.ExitCode}
 $p.Dispose()
 if($result.ExitCode -ne 0){throw "$Exe failed ($($result.ExitCode)): $($result.Stderr)"}
 return $result
}
function Linux([string[]]$Arguments){return Native 'wsl.exe' (@('--distribution',$Distro,'--user','root','--exec')+$Arguments)}
function Adapter([string[]]$Arguments){return Native $WindowsCLI (@('wsl','--distro',$Distro,'--linux-cli',$LinuxCLI)+$Arguments)}
function Memory {
 $processes=@(Get-Process | Where-Object ProcessName -match '^(vmmem|Docker Desktop|com\.docker\.|docker-agent)' | ForEach-Object {
  [pscustomobject]@{Name=$_.ProcessName;PID=$_.Id;WorkingSetBytes=$_.WorkingSet64;PrivateBytes=$_.PrivateMemorySize64}
 })
 $os=Get-CimInstance Win32_OperatingSystem
 return [pscustomobject]@{UTC=[DateTime]::UtcNow.ToString('o');HostFreePhysicalKiB=$os.FreePhysicalMemory;Processes=$processes}
}
function Invoke-Measurement([string]$Name,[int]$Iteration,[scriptblock]$Body){
 $before=Memory
 $result=& $Body
 $after=Memory
 $row=[pscustomobject]@{Operation=$Name;Iteration=$Iteration;Milliseconds=$result.Milliseconds;Stdout=$result.Stdout;Stderr=$result.Stderr;Before=$before;After=$after}
 $script:rows.Add($row)
 # Save after every operation so failed experiments retain their observations.
 $script:rows | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Artifacts 'raw.json') -Encoding utf8
 return $result
}
function Disk {
 $vhd=@()
 foreach($base in @((Join-Path $env:LOCALAPPDATA 'PocketLinxVerification/distro'),(Join-Path $env:LOCALAPPDATA 'Docker/wsl'))){
  if(Test-Path -LiteralPath $base){$vhd+=@(Get-ChildItem -LiteralPath $base -Filter '*.vhdx' -Recurse | Select-Object FullName,Length)}
 }
 return [pscustomobject]@{VHDXFileLengths=$vhd;DockerSystemDF=(Native 'docker.exe' @('system','df','--format','{{json .}}')).Stdout;WindowsArtifactFileBytes=(Get-ChildItem -LiteralPath $Artifacts -Recurse -File | Measure-Object Length -Sum).Sum}
}
$metadata=[ordered]@{
 StartedUTC=[DateTime]::UtcNow.ToString('o');Id=$id;GitCommit=(Native 'git.exe' @('rev-parse','HEAD')).Stdout.Trim()
 CPU=Get-CimInstance Win32_Processor | Select-Object Name,NumberOfCores,NumberOfLogicalProcessors
 RAMBytes=(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory
 OS=Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber
 DockerVersion=(Native 'docker.exe' @('version','--format','{{json .}}')).Stdout | ConvertFrom-Json
 ExistingRunningContainerCount=((Native 'docker.exe' @('ps','-q')).Stdout.Trim() -split "`n" | Where-Object {$_}).Count
 WindowsCLISize=(Get-Item -LiteralPath $WindowsCLI).Length;WindowsCLISHA256=(Get-FileHash -LiteralPath $WindowsCLI).Hash
 Distro=$Distro;LinuxRoot=$linuxRoot;Repetitions=$Repetitions;ColdStart='unmeasured: existing WSL/Docker workloads are not stopped'
}
$metadata.WSLVersion=(Native 'wsl.exe' @('--version')).Stdout.Replace([string][char]0,'')
$metadata.WSLListing=(Native 'wsl.exe' @('--list','--verbose')).Stdout.Replace([string][char]0,'')
$metadata.DiskBefore=Disk
Linux @('/bin/mkdir',$linuxRoot) | Out-Null
Linux @($Fixture,'create',"$linuxRoot/environment",$Sample) | Out-Null
$definition=(Linux @('/bin/cat',"$linuxRoot/environment/environment.json")).Stdout | ConvertFrom-Json
$metadata.Definition=$definition
$metadata.LinuxKernel=(Linux @('/bin/uname','-a')).Stdout.Trim()
$metadata.LinuxCLI=(Linux @('/bin/busybox','sha256sum',$LinuxCLI)).Stdout.Trim()
$metadata.LinuxCLIBytes=[long](Linux @('/bin/busybox','stat','-c','%s',$LinuxCLI)).Stdout.Trim()
$metadata.LinuxMemoryBefore=(Linux @('/bin/cat','/proc/meminfo')).Stdout
$metadata.LinuxFilesystems=(Linux @('/bin/busybox','df','-k',$linuxRoot)).Stdout
$baseBundle=Join-Path $Artifacts 'initial.plxenv'
Adapter @('--stopped','save',"$linuxRoot/environment",$baseBundle) | Out-Null
# Minimal Docker rootfs has identical binaries/source/data, but embeds data in an image.
Linux @('/bin/cp','-a',"$linuxRoot/environment/rootfs","$linuxRoot/docker-root") | Out-Null
Linux @('/bin/cp','-a',"$linuxRoot/environment/source/.",("$linuxRoot/docker-root"+$definition.source.target+'/')) | Out-Null
foreach($volume in $definition.volumes){Linux @('/bin/cp','-a',"$linuxRoot/environment/volumes/$($volume.name)/.",("$linuxRoot/docker-root"+$volume.target+'/')) | Out-Null}
Linux @('/bin/tar','-cf',"$LinuxArtifacts/rootfs.tar",'-C',"$linuxRoot/docker-root",'.') | Out-Null
$image="$id`:base"
$changes=@('--change',"USER $($definition.uid):$($definition.gid)",'--change',"WORKDIR $($definition.workdir)",'--change',('CMD '+(ConvertTo-Json -InputObject @($definition.command) -Compress)))
foreach($property in $definition.env.PSObject.Properties){$changes+=@('--change',"ENV $($property.Name)=$($property.Value)")}
Native 'docker.exe' (@('import','--platform','linux/amd64')+$changes+@((Join-Path $Artifacts 'rootfs.tar'),$image)) | Out-Null
$created.Add($image)
$expected='{0}|{1}|persistent-data|uid={2}|gid={3}' -f $definition.env.GREETING,$definition.workdir,$definition.uid,$definition.gid
# Explicit warmups, excluded from timing. Docker uses a fresh, automatically removed container per run.
$warmA=Adapter @('--trusted-sample','run',"$linuxRoot/environment")
$warmB=Native 'docker.exe' @('run','--rm','--network','none',$image)
if($warmA.Stdout.Trim() -ne $expected -or $warmB.Stdout.Trim() -ne $expected){throw 'Workloads differ'}
for($i=1;$i -le $Repetitions;$i++){
 # Alternate order to reduce a fixed-order bias.
 if($i%2){$a=Invoke-Measurement 'pocket_run_warm' $i {Adapter @('--trusted-sample','run',"$linuxRoot/environment")};$b=Invoke-Measurement 'docker_run_warm' $i {Native 'docker.exe' @('run','--rm','--network','none',$image)}}
 else{$b=Invoke-Measurement 'docker_run_warm' $i {Native 'docker.exe' @('run','--rm','--network','none',$image)};$a=Invoke-Measurement 'pocket_run_warm' $i {Adapter @('--trusted-sample','run',"$linuxRoot/environment")}}
 if($a.Stdout.Trim() -ne $expected -or $b.Stdout.Trim() -ne $expected){throw 'Measured output differs'}
 Invoke-Measurement 'pocket_restore_transfer' $i {Adapter @('restore',$baseBundle,"$linuxRoot/restored-$i")} | Out-Null
 Invoke-Measurement 'pocket_linux_cli_rss' $i {Linux @('/bin/busybox','time','-v',$LinuxCLI,'run',"$linuxRoot/environment")} | Out-Null
 Invoke-Measurement 'docker_payload_rss' $i {Native 'docker.exe' (@('run','--rm','--network','none','--entrypoint','/bin/busybox',$image,'time','-v')+@($definition.command))} | Out-Null
}
# Both receivers change the same source and data. Pocket data stays outside rootfs;
# Docker data here stays in the container writable layer, not named volumes.
Linux @($Fixture,'edit',"$linuxRoot/restored-1",$Sample) | Out-Null
Adapter @('--trusted-sample','run',"$linuxRoot/restored-1") | Out-Null
$container="$id-changed"
Native 'docker.exe' @('create','--name',$container,'--network','none',$image) | Out-Null
$created.Add($container)
Linux @('/bin/cp',"$linuxRoot/restored-1/source/hello.sh","$LinuxArtifacts/hello.sh") | Out-Null
Linux @('/bin/cp',"$linuxRoot/restored-1/source/generated.txt","$LinuxArtifacts/generated.txt") | Out-Null
Linux @('/bin/cp',"$linuxRoot/restored-1/volumes/data/message.txt","$LinuxArtifacts/message.txt") | Out-Null
# cp ownership on Windows is not a Linux ownership claim; fix only benchmark container files explicitly.
foreach($pair in @(@('hello.sh',($definition.source.target+'/hello.sh')),@('generated.txt',($definition.source.target+'/generated.txt')),@('message.txt',$definition.env.DATA_FILE))){Native 'docker.exe' @('cp',(Join-Path $Artifacts $pair[0]),("${container}:"+$pair[1])) | Out-Null}
# Use a benchmark-only root command to prepare ownership; never modify any existing container.
$setupImage="$id`:prepared"
Native 'docker.exe' @('commit',$container,$setupImage) | Out-Null
$created.Add($setupImage)
$fixContainer="$id-owned"
Native 'docker.exe' @('run','--name',$fixContainer,'--network','none','--user','0:0','--entrypoint','/bin/busybox',$setupImage,'chown','-R',"$($definition.uid):$($definition.gid)",$definition.source.target,$definition.volumes[0].target) | Out-Null
$created.Add($fixContainer)
for($i=1;$i -le $Repetitions;$i++){
 $bundle=Join-Path $Artifacts "changed-$i.plxenv"
 Invoke-Measurement 'pocket_resave_transfer' $i {Adapter @('--stopped','save',"$linuxRoot/restored-1",$bundle)} | Out-Null
 $savedImage="$id`:saved-$i"
 Invoke-Measurement 'docker_commit' $i {Native 'docker.exe' @('commit','--change',"USER $($definition.uid):$($definition.gid)",'--change',('CMD '+(ConvertTo-Json -InputObject @($definition.command) -Compress)),'--change','ENTRYPOINT []',$fixContainer,$savedImage)} | Out-Null
 $created.Add($savedImage)
 $archive=Join-Path $Artifacts "docker-$i.tar"
 Invoke-Measurement 'docker_image_save' $i {Native 'docker.exe' @('save','--output',$archive,$savedImage)} | Out-Null
 Invoke-Measurement 'docker_image_load_cached' $i {Native 'docker.exe' @('load','--input',$archive)} | Out-Null
 $out=Native 'docker.exe' @('run','--rm','--network','none',$savedImage)
 if($out.Stdout.Trim() -ne ($expected.Replace('persistent-data','runtime-write')+"`nedited")){throw 'Saved Docker state differs'}
}
$metadata.LinuxMemoryAfter=(Linux @('/bin/cat','/proc/meminfo')).Stdout
$metadata.LinuxDataUsage=@($linuxRoot,"$linuxRoot/environment","$linuxRoot/restored-1" | ForEach-Object {(Linux @('/bin/busybox','du','-sk',$_)).Stdout})
$metadata.LinuxResidualTemporaryFiles=(Linux @('/bin/busybox','find',$linuxRoot,'-name','.plx-*')).Stdout
$metadata.WindowsResidualTemporaryFiles=@(Get-ChildItem -LiteralPath $Artifacts -Filter '.plx-*' | Select-Object Name,Length)
$metadata.DockerCLIBytes=(Get-Item -LiteralPath (Get-Command docker.exe).Source).Length
$metadata.LinuxDistroDiskUsage=(Linux @('/bin/busybox','du','-skx','/')).Stdout
$dockerInstall=Split-Path (Split-Path (Split-Path (Get-Command docker.exe).Source))
$metadata.DockerInstallFileBytes=(Get-ChildItem -LiteralPath $dockerInstall -Recurse -File | Measure-Object Length -Sum).Sum
$metadata.DockerImageInspect=(Native 'docker.exe' @('image','inspect',"$id`:saved-1")).Stdout | ConvertFrom-Json
$metadata.DockerContainerInspect=(Native 'docker.exe' @('container','inspect','--size',$fixContainer)).Stdout | ConvertFrom-Json
$metadata.Packages=@(Get-ChildItem -LiteralPath $Artifacts -File | Where-Object Extension -in '.plxenv','.tar' | ForEach-Object {[pscustomobject]@{Name=$_.Name;Bytes=$_.Length;SHA256=(Get-FileHash -LiteralPath $_.FullName).Hash}})
$metadata.DiskAfter=Disk
$metadata.CreatedDockerResources=@($created)
$metadata | ConvertTo-Json -Depth 15 | Set-Content -LiteralPath (Join-Path $Artifacts 'metadata.json') -Encoding utf8
$summary=@($rows | Group-Object Operation | ForEach-Object {
 $values=@($_.Group.Milliseconds | Sort-Object)
 $median=if($values.Count%2){$values[[int][math]::Floor($values.Count/2)]}else{($values[$values.Count/2-1]+$values[$values.Count/2])/2}
 [pscustomobject]@{Operation=$_.Name;MedianMilliseconds=$median;ValuesMilliseconds=@($_.Group.Milliseconds)}
})
$summary | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $Artifacts 'summary.json') -Encoding utf8
$summary | Format-Table Operation,MedianMilliseconds
Write-Output "Artifacts retained at $Artifacts; Docker prefix $id; Linux root $linuxRoot"
