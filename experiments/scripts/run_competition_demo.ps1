param(
    [string]$BaseUrl = "http://127.0.0.1:8088",
    [switch]$RunIsolatedUnlearning,
    [switch]$OpenReport
)

$ErrorActionPreference = "Stop"
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\.."))
$Experiments = Join-Path $ProjectRoot "experiments"
$Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$RunDir = Join-Path $Experiments "results\demo_runs\$Timestamp"
New-Item -ItemType Directory -Force $RunDir | Out-Null

if (-not $env:EVAL_USER_ID -or -not $env:EVAL_PASSWORD) {
    throw "Set EVAL_USER_ID and EVAL_PASSWORD before the protected demo."
}

Write-Host "[1/5] Health check"
$health = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 "$BaseUrl/health"
if ($health.StatusCode -ne 200) { throw "Health check failed: HTTP $($health.StatusCode)" }

Write-Host "[2/5] Real-time retrieval trace"
python (Join-Path $PSScriptRoot "run_trace_queries.py") --mode rrf --limit 1 --base-url $BaseUrl --output (Join-Path $RunDir "retrieval_trace.json")
if ($LASTEXITCODE -ne 0) { throw "Retrieval trace failed" }

Write-Host "[3/5] Retrieval latency gate (warm-up excluded)"
python (Join-Path $PSScriptRoot "measure_retrieval_latency.py") --mode rrf --count 10 --warmup 3 --base-url $BaseUrl --output (Join-Path $RunDir "retrieval_latency.json")
if ($LASTEXITCODE -ne 0) { throw "Retrieval P95 did not pass the 1s gate" }

Write-Host "[4/5] Lightweight input-security check"
python (Join-Path $PSScriptRoot "security_eval.py") --representative --base-url $BaseUrl --config-label full_system | Tee-Object -FilePath (Join-Path $RunDir "security.log")
if ($LASTEXITCODE -ne 0) { throw "Security representative check failed" }

if ($RunIsolatedUnlearning) {
    if ($env:EVAL_DEMO_ALLOW_UNLEARNING -ne "true") {
        throw "Set EVAL_DEMO_ALLOW_UNLEARNING=true before requesting destructive isolated-user demonstration."
    }
    Write-Host "[5/5] Isolated unlearning"
    python (Join-Path $PSScriptRoot "unlearning_eval.py") --trials 1 --base-url $BaseUrl | Tee-Object -FilePath (Join-Path $RunDir "unlearning.log")
    if ($LASTEXITCODE -ne 0) { throw "Isolated unlearning demonstration failed" }
} else {
    Write-Host "[5/5] Unlearning skipped: enable only after reseeding its isolated target scope."
}

$report = Join-Path $Experiments "results\reports\full_report.html"
Copy-Item $report (Join-Path $RunDir "full_report.html") -ErrorAction SilentlyContinue
Write-Host "Causal evidence is shown from the pre-run report: $report"
Write-Host "Demo artifacts: $RunDir"
if ($OpenReport -and (Test-Path $report)) { Start-Process $report }
