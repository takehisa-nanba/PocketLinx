# Offline validation only; never launches Docker/WSL or performs a benchmark.
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot -Parent
$data=Join-Path $repo 'docs/benchmarks/2026-09-24'
$scratch=Join-Path ([IO.Path]::GetTempPath()) ('plx-stat-test-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratch | Out-Null
try {
 foreach($file in 'measure-baseline.ps1','summarize-baseline.ps1'){
  $tokens=$null;$errors=$null
  [void][Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $file),[ref]$tokens,[ref]$errors)
  if($errors.Count){throw "PowerShell parse errors: $errors"}
 }
 $out=Join-Path $scratch 'stats.json'
 & (Join-Path $PSScriptRoot 'summarize-baseline.ps1') -Raw (Join-Path $data 'raw.json') -Output $out
 $stats=Get-Content -Raw $out | ConvertFrom-Json
 $reference=Get-Content -Raw (Join-Path $data 'summary.json') | ConvertFrom-Json
 foreach($expected in $reference){
  $actual=$stats | Where-Object Operation -eq $expected.Operation
  if($actual.Milliseconds.ValidCount -ne 5 -or [math]::Abs($actual.Milliseconds.Median-$expected.MedianMilliseconds) -gt 0.000001){throw "Median mismatch: $($expected.Operation)"}
 }
 # A missing VM observation must not turn into a fabricated zero measurement.
 $synthetic=@(Get-Content -Raw (Join-Path $data 'raw.json') | ConvertFrom-Json)[0]
 $synthetic.Before.Processes=@();$synthetic.After.Processes=@()
 $inputFile=Join-Path $scratch 'missing.json';@($synthetic) | ConvertTo-Json -Depth 8 | Set-Content $inputFile
 & (Join-Path $PSScriptRoot 'summarize-baseline.ps1') -Raw $inputFile -Output $out
 $missing=Get-Content -Raw $out | ConvertFrom-Json
 if($null -ne $missing.SharedVMWorkingSetAfterMiB){throw 'Missing VM became a value'}
 Write-Output 'PASS: parser, nine five-run medians, missing-observation handling'
} finally {
 # Only these known files in this newly-created test directory are removed.
 foreach($name in 'stats.json','missing.json') { $file=Join-Path $scratch $name; if(Test-Path -LiteralPath $file){Remove-Item -LiteralPath $file} }
 Remove-Item -LiteralPath $scratch
}
