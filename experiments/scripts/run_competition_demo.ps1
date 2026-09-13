param(
    [string]$BaseUrl = "http://127.0.0.1:8088",
    [ValidateSet("ollama", "fixture")]
    [string]$Mode = "ollama",
    [switch]$RunIsolatedUnlearning,
    [switch]$OpenReport,
    [switch]$OpenConsole
)

$ErrorActionPreference = "Stop"
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\.."))
$Experiments = Join-Path $ProjectRoot "experiments"
$Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$RunDir = Join-Path $Experiments "results\demo_runs\$Timestamp"
New-Item -ItemType Directory -Force $RunDir | Out-Null
$isFixture = $Mode -eq "fixture"
$consolePath = Join-Path $ProjectRoot "web\competition_console.html"
$ConsoleUrl = if ($isFixture) {
    "$([System.Uri]::new($consolePath).AbsoluteUri)?mode=fixture"
} else {
    "$($BaseUrl.TrimEnd('/'))/web/competition_console.html?mode=$Mode"
}
$health = [ordered]@{ status_code = $null; passed = $false }
$authCheck = [ordered]@{ checked = $false; credential_source = "none_fixture"; token_persisted = $false }

if ($isFixture) {
    Write-Host "[1/6] Offline fixture mode: no service, credentials, model, or evaluation API required"
    $modelCheck = [ordered]@{ mode = "fixture"; status = "fixed_sample_only"; model = $null; host = $null }
} else {
    if (-not $env:EVAL_USER_ID -or -not $env:EVAL_PASSWORD) {
        throw "Set EVAL_USER_ID and EVAL_PASSWORD before the protected demo."
    }

    Write-Host "[1/6] Health check"
    $healthResponse = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 "$BaseUrl/health"
    if ($healthResponse.StatusCode -ne 200) { throw "Health check failed: HTTP $($healthResponse.StatusCode)" }
    $health = [ordered]@{ status_code = $healthResponse.StatusCode; passed = $true }

    Write-Host "[2/6] Protected authentication check"
    $loginBody = @{ user_id = $env:EVAL_USER_ID; password = $env:EVAL_PASSWORD; workspace_id = $(if ($env:EVAL_WORKSPACE_ID) { $env:EVAL_WORKSPACE_ID } else { "default" }) } | ConvertTo-Json -Compress
    $login = Invoke-RestMethod -Method Post -ContentType "application/json" -TimeoutSec 10 -Uri "$BaseUrl/api/auth/login" -Body $loginBody
    if (-not $login.success -or -not $login.data.token) { throw "Authentication check did not return a token." }
    $loginBody = $null
    $login = $null
    $authCheck = [ordered]@{ checked = $true; credential_source = "environment"; token_persisted = $false }

    $modelCheck = [ordered]@{ mode = $Mode; status = "not_checked"; model = $env:OLLAMA_MODEL; host = $env:OLLAMA_HOST }
    Write-Host "[3/6] Ollama model availability check"
    $ollamaHost = if ($env:OLLAMA_HOST) { $env:OLLAMA_HOST.TrimEnd('/') } else { "http://127.0.0.1:11434" }
    $ollamaModel = if ($env:OLLAMA_MODEL) { $env:OLLAMA_MODEL } else { "qwen2.5:3b" }
    try {
        $tags = Invoke-RestMethod -UseBasicParsing -TimeoutSec 5 "$ollamaHost/api/tags"
        $names = @($tags.models | ForEach-Object { $_.name })
        if ($names -notcontains $ollamaModel) { throw "Model '$ollamaModel' is not installed. Available: $($names -join ', ')" }
        $modelCheck = [ordered]@{ mode = "ollama"; status = "available"; model = $ollamaModel; host = $ollamaHost }
    } catch {
        throw "Ollama check failed: $($_.Exception.Message). Use -Mode fixture only for the clearly labelled fixed offline sample."
    }
}

$manifest = [ordered]@{
    schema = "competition-demo-run-v1"
    timestamp_local = (Get-Date).ToString("o")
    base_url = $BaseUrl
    console_url = $ConsoleUrl
    health = $health
    authentication = $authCheck
    model = $modelCheck
    isolated_unlearning_requested = [bool]$RunIsolatedUnlearning
    artifacts = $(if ($isFixture) { ,@("run_manifest.json") } else { ,@("run_manifest.json", "retrieval_trace.json", "retrieval_latency.json", "security.log") })
}
$manifest | ConvertTo-Json -Depth 8 | Set-Content -Encoding UTF8 (Join-Path $RunDir "run_manifest.json")

if (-not $isFixture) {
    Write-Host "[4/6] Real-time retrieval trace"
    python (Join-Path $PSScriptRoot "run_trace_queries.py") --mode rrf --limit 1 --base-url $BaseUrl --output (Join-Path $RunDir "retrieval_trace.json")
    if ($LASTEXITCODE -ne 0) { throw "Retrieval trace failed" }

    Write-Host "[5/6] Retrieval latency gate (warm-up excluded)"
    python (Join-Path $PSScriptRoot "measure_retrieval_latency.py") --mode rrf --count 10 --warmup 3 --base-url $BaseUrl --output (Join-Path $RunDir "retrieval_latency.json")
    if ($LASTEXITCODE -ne 0) { throw "Retrieval P95 did not pass the 1s gate" }

    Write-Host "[6/6] Lightweight input-security check"
    python (Join-Path $PSScriptRoot "security_eval.py") --representative --base-url $BaseUrl --config-label full_system | Tee-Object -FilePath (Join-Path $RunDir "security.log")
    if ($LASTEXITCODE -ne 0) { throw "Security representative check failed" }
}

if ($RunIsolatedUnlearning -and $isFixture) {
    throw "Isolated unlearning requires ollama/live API mode; fixture mode never performs deletion."
}
if ($RunIsolatedUnlearning) {
    if ($env:EVAL_DEMO_ALLOW_UNLEARNING -ne "true") {
        throw "Set EVAL_DEMO_ALLOW_UNLEARNING=true before requesting destructive isolated-user demonstration."
    }
    Write-Host "[post-check] Isolated unlearning"
    python (Join-Path $PSScriptRoot "unlearning_eval.py") --trials 1 --base-url $BaseUrl | Tee-Object -FilePath (Join-Path $RunDir "unlearning.log")
    if ($LASTEXITCODE -ne 0) { throw "Isolated unlearning demonstration failed" }
} else {
    Write-Host "[post-check] Unlearning skipped: enable only after reseeding its isolated target scope."
}

$report = Join-Path $Experiments "results\reports\full_report.html"
Copy-Item $report (Join-Path $RunDir "full_report.html") -ErrorAction SilentlyContinue
Write-Host "Causal evidence is shown from the pre-run report: $report"
Write-Host "Demo artifacts: $RunDir"
Write-Host "Competition console: $ConsoleUrl"
if ($OpenConsole) { Start-Process $ConsoleUrl }
if ($OpenReport -and (Test-Path $report)) { Start-Process $report }
