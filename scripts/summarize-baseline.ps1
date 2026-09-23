param([Parameter(Mandatory=$true)][string]$Raw,[Parameter(Mandatory=$true)][string]$Output)
$ErrorActionPreference='Stop'
function Statistics([object[]]$Values){
 $sorted=@($Values | Where-Object {$null -ne $_} | ForEach-Object {[double]$_} | Sort-Object);$n=$sorted.Count
 if($n -eq 0){return $null}
 $median=if($n%2){$sorted[[int][math]::Floor($n/2)]}else{($sorted[$n/2-1]+$sorted[$n/2])/2}
 return [pscustomobject]@{Values=$Values;ValidCount=$n;Median=$median;Minimum=$sorted[0];Maximum=$sorted[-1]}
}
function ObservedWorkingSet($Processes){
 if(@($Processes).Count -eq 0){return $null}
 return ($Processes | Measure-Object WorkingSetBytes -Sum).Sum/1MB
}
$rows=@(Get-Content -Raw -LiteralPath $Raw | ConvertFrom-Json)
$result=@($rows | Group-Object Operation | ForEach-Object {
 $group=$_.Group
 $rss=@($group | ForEach-Object {if($_.Stderr -match 'Maximum resident set size \(kbytes\): (\d+)'){[double]$Matches[1]}})
 $beforeVM=@($group | ForEach-Object {ObservedWorkingSet @($_.Before.Processes | Where-Object Name -like 'vmmem*')})
 $afterVM=@($group | ForEach-Object {ObservedWorkingSet @($_.After.Processes | Where-Object Name -like 'vmmem*')})
 $docker=@($group | ForEach-Object {ObservedWorkingSet @($_.After.Processes | Where-Object Name -notlike 'vmmem*')})
 [pscustomobject]@{
  Operation=$_.Name;Milliseconds=Statistics @($group.Milliseconds)
  ProcessMaxRSSKiB=if($rss.Count){Statistics $rss}else{$null}
  SharedVMWorkingSetBeforeMiB=Statistics $beforeVM;SharedVMWorkingSetAfterMiB=Statistics $afterVM
  DockerWindowsProcessesWorkingSetAfterMiB=Statistics $docker
  HostFreePhysicalBeforeMiB=Statistics @($group | ForEach-Object {$_.Before.HostFreePhysicalKiB/1024})
  HostFreePhysicalAfterMiB=Statistics @($group | ForEach-Object {$_.After.HostFreePhysicalKiB/1024})
 }
})
$result | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Output -Encoding utf8
