<#
Run exactly one evaluator against one temporary runtime configuration on Windows.
The runner restores config/.env even when the evaluator fails and marks only a
single, successful raw result as eligible for a report.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('baseline_vanilla_llm', 'baseline_naive_rag', 'baseline_rag_with_filter', 'full_system')]
    [string]$ConfigName,

    [string]$EvalCommand,

    [string]$EncodedEvalCommand
)

$ErrorActionPreference = 'Stop'
$env:PYTHONIOENCODING = 'utf-8'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
if ($EncodedEvalCommand) {
    try {
        $EvalCommand = [System.Text.Encoding]::Unicode.GetString([System.Convert]::FromBase64String($EncodedEvalCommand))
    } catch {
        throw 'EncodedEvalCommand must be Base64-encoded UTF-16LE text.'
    }
}
if ([string]::IsNullOrWhiteSpace($EvalCommand)) {
    throw 'Specify EvalCommand or EncodedEvalCommand.'
}
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$RuntimeEnv = Join-Path $ProjectRoot "experiments/runtime_env/$ConfigName.env"
$TargetEnv = Join-Path $ProjectRoot 'config/.env'
$ResultDir = Join-Path $ProjectRoot 'experiments/results/raw'
$RunId = "{0}_{1}_{2}" -f (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ'), $ConfigName, $PID
$RunDir = Join-Path $ProjectRoot "experiments/results/runs/$RunId"
$BackupEnv = Join-Path $RunDir 'base.env.backup'
$EffectiveEnv = Join-Path $RunDir 'effective.env'
$Manifest = Join-Path $RunDir 'manifest.json'
$SmokeLog = Join-Path $RunDir 'smoke.log'
$EvalLog = Join-Path $RunDir 'evaluation.log'
$Marker = Join-Path $RunDir 'evaluation.started'
$StartedAt = (Get-Date).ToUniversalTime().ToString('o')
$Outcome = 'initializing'
$Failure = $null
$SmokeStatus = 'not_run'
$EvalExitCode = $null
$ResultFile = $null

function Get-Sha256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Get-DatasetTreeHash([string]$Root) {
    $builder = [System.Text.StringBuilder]::new()
    Get-ChildItem -LiteralPath $Root -Recurse -File | Sort-Object FullName | ForEach-Object {
        $relative = $_.FullName.Substring($Root.Length).TrimStart('\', '/')
        [void]$builder.Append($relative.Replace('\\', '/'))
        [void]$builder.Append(':')
        [void]$builder.Append((Get-Sha256 $_.FullName))
        [void]$builder.Append("`n")
    }
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($builder.ToString())
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { return ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Merge-EnvFiles([string]$BasePath, [string]$OverlayPath, [string]$OutputPath) {
    $overlay = @{}
    foreach ($line in Get-Content -LiteralPath $OverlayPath -Encoding utf8) {
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith('#') -and $trimmed.Contains('=')) {
            $overlay[$trimmed.Split('=', 2)[0].Trim()] = $line
        }
    }
    $lines = [System.Collections.Generic.List[string]]::new()
    $seen = @{}
    foreach ($line in Get-Content -LiteralPath $BasePath -Encoding utf8) {
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith('#') -and $trimmed.Contains('=')) {
            $key = $trimmed.Split('=', 2)[0].Trim()
            if ($overlay.ContainsKey($key)) {
                $lines.Add($overlay[$key])
                $seen[$key] = $true
                continue
            }
        }
        $lines.Add($line)
    }
    foreach ($line in Get-Content -LiteralPath $OverlayPath -Encoding utf8) {
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith('#') -and $trimmed.Contains('=')) {
            $key = $trimmed.Split('=', 2)[0].Trim()
            if (-not $seen.ContainsKey($key)) { $lines.Add($line); $seen[$key] = $true }
        }
    }
    [System.IO.File]::WriteAllLines($OutputPath, $lines, [System.Text.UTF8Encoding]::new($false))
}

function Write-Manifest {
    $payload = [ordered]@{
        schema_version = 1
        run_id = $RunId
        started_at = $StartedAt
        ended_at = (Get-Date).ToUniversalTime().ToString('o')
        outcome = $Outcome
        error = $Failure
        smoke_status = $SmokeStatus
        evaluator_exit_code = $EvalExitCode
        result_file = $ResultFile
        command = $EvalCommand
        command_sha256 = $CommandHash
        configuration = [ordered]@{ name = $ConfigName; base_env_sha256 = $BaseHash; overlay_env_sha256 = $OverlayHash; effective_env_sha256 = $EffectiveHash }
        data = [ordered]@{ datasets_tree_sha256 = $DatasetHash }
        environment = [ordered]@{ powershell = $PSVersionTable.PSVersion.ToString(); docker_compose = $ComposeVersion; platform = [System.Environment]::OSVersion.VersionString; health_url = 'http://localhost:8088/health' }
        artifacts = [ordered]@{ smoke_log = $SmokeLog; evaluation_log = $EvalLog }
    }
    $manifestJson = $payload | ConvertTo-Json -Depth 6
    [System.IO.File]::WriteAllText($Manifest, $manifestJson + "`n", [System.Text.UTF8Encoding]::new($false))
}

New-Item -ItemType Directory -Force -Path $RunDir, $ResultDir | Out-Null
if (-not (Test-Path -LiteralPath $RuntimeEnv) -or -not (Test-Path -LiteralPath $TargetEnv)) { throw 'Required base or overlay environment file is missing.' }

$BaseHash = Get-Sha256 $TargetEnv
$OverlayHash = Get-Sha256 $RuntimeEnv
$DatasetHash = Get-DatasetTreeHash (Join-Path $ProjectRoot 'experiments/datasets')
$CommandHash = [System.BitConverter]::ToString(([System.Security.Cryptography.SHA256]::Create().ComputeHash([System.Text.Encoding]::UTF8.GetBytes($EvalCommand)))).Replace('-', '').ToLowerInvariant()
$ComposeVersion = (& docker-compose version 2>&1 | Out-String).Trim()

Copy-Item -LiteralPath $TargetEnv -Destination $BackupEnv -Force
try {
    Merge-EnvFiles $TargetEnv $RuntimeEnv $EffectiveEnv
    $EffectiveHash = Get-Sha256 $EffectiveEnv
    Copy-Item -LiteralPath $EffectiveEnv -Destination $TargetEnv -Force

    Push-Location $ProjectRoot
    # docker-compose writes progress lines to stderr, which Windows PowerShell 5.1
    # promotes to a terminating error under $ErrorActionPreference = 'Stop'.
    # Wrapping in cmd.exe keeps the exit code while avoiding that false failure.
    cmd /c "docker-compose up -d --force-recreate --no-deps context-keeper >nul 2>&1"
    if ($LASTEXITCODE -ne 0) { throw "docker-compose restart failed with exit code $LASTEXITCODE" }
    $healthy = $false
    foreach ($unused in 1..60) {
        try {
            if ((Invoke-WebRequest -UseBasicParsing -TimeoutSec 3 'http://127.0.0.1:8088/health').StatusCode -eq 200) { $healthy = $true; break }
        } catch {}
        Start-Sleep -Seconds 1
    }
    if (-not $healthy) { throw 'service did not become healthy within 60 seconds' }

    & python experiments/scripts/smoke_test.py --quick *>&1 | Tee-Object -FilePath $SmokeLog
    if ($LASTEXITCODE -ne 0) { $SmokeStatus = 'failed'; throw 'non-destructive smoke test failed' }
    $SmokeStatus = 'passed'

    New-Item -ItemType File -Path $Marker -Force | Out-Null
    $env:EVAL_RUNTIME_CONFIG_NAME = $ConfigName
    $env:EVAL_RUNTIME_CONFIG_FILE = $RuntimeEnv
    $env:EVAL_RUNTIME_CONFIG_HASH = $EffectiveHash
    $env:EVAL_SERVICE_START_TIME = $StartedAt
    $env:EVAL_RUN_ID = $RunId
    $env:EVAL_DATASETS_TREE_HASH = $DatasetHash
    # Child evaluators use this attestation to ensure their recorded
    # configuration name matches the runtime this script has just restarted.
    $env:EVAL_COMPETITION_RUNTIME_READY = $ConfigName
    Invoke-Expression $EvalCommand *>&1 | Tee-Object -FilePath $EvalLog
    $EvalExitCode = $LASTEXITCODE

    $results = @(Get-ChildItem -LiteralPath $ResultDir -Filter '*.json' -File | Where-Object { $_.LastWriteTimeUtc -gt (Get-Item -LiteralPath $Marker).LastWriteTimeUtc })
    if ($results.Count -ne 1) { throw "expected exactly one new raw JSON result, found $($results.Count)" }
    $ResultFile = $results[0].FullName
    $Outcome = if ($EvalExitCode -eq 0) { 'succeeded' } else { 'evaluator_failed' }
    if ($EvalExitCode -ne 0) { $Failure = "evaluator exited with code $EvalExitCode" }

    $result = Get-Content -LiteralPath $ResultFile -Raw -Encoding utf8 | ConvertFrom-Json
    $provenance = [pscustomobject][ordered]@{
        run_id = $RunId; manifest = $Manifest; outcome = $Outcome; report_eligible = ($Outcome -eq 'succeeded'); error = $Failure
        configuration = [ordered]@{ name = $ConfigName; base_env_sha256 = $BaseHash; overlay_env_sha256 = $OverlayHash; effective_env_sha256 = $EffectiveHash }
        data = [ordered]@{ datasets_tree_sha256 = $DatasetHash }
        environment = [ordered]@{ powershell = $PSVersionTable.PSVersion.ToString(); docker_compose = $ComposeVersion; platform = [System.Environment]::OSVersion.VersionString }
        evaluator_exit_code = $EvalExitCode; smoke_status = $SmokeStatus
    }
    $result | Add-Member -NotePropertyName runner_provenance -NotePropertyValue $provenance -Force
    $resultJson = $result | ConvertTo-Json -Depth 16
    [System.IO.File]::WriteAllText($ResultFile, $resultJson + "`n", [System.Text.UTF8Encoding]::new($false))
    if ($EvalExitCode -ne 0) { throw $Failure }
    Write-Output "Eligible result: $ResultFile"
}
catch {
    if ($Outcome -eq 'initializing') { $Outcome = 'runner_failed' }
    if (-not $Failure) { $Failure = $_.Exception.Message }
    throw
}
finally {
    Copy-Item -LiteralPath $BackupEnv -Destination $TargetEnv -Force
    Write-Manifest
    Push-Location $ProjectRoot
    # Same Windows PowerShell 5.1 stderr promotion as above; exit code is not
    # consumed here, so the output is simply discarded inside cmd.exe.
    cmd /c "docker-compose up -d --force-recreate --no-deps context-keeper >nul 2>&1"
    Pop-Location
}
