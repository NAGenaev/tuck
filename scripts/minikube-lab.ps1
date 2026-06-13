# Tuck minikube lab — local K8s for UI + operator testing.
#
#   .\scripts\minikube-lab.ps1                 # kubectl manifests (default)
#   .\scripts\minikube-lab.ps1 -UseHelm        # Helm chart + values-minikube.yaml
#   .\scripts\minikube-lab.ps1 -SmokeOnly      # repeat TuckSecret demo
#   .\scripts\minikube-lab.ps1 -SkipBuild      # skip docker build

param(
    [switch]$UseHelm,
    [switch]$SmokeOnly,
    [switch]$SkipBuild,
    [switch]$ResetData   # delete PVC — new ROOT TOKEN in logs
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$TokenFile = Join-Path $Root "testdata\minikube-root-token.txt"
$Ns = "tuck"
$LabPort = 8201   # 8200 часто занят локальным tuck server -dev
$script:DeployMode = "kubectl"

function Write-Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }

function Start-TuckPortForward {
    $svc = if ($script:DeployMode -eq "helm") { "tuck-server" } else { "tuck" }
    $job = Start-Job -ScriptBlock {
        param($namespace, $service, $port)
        kubectl -n $namespace port-forward "svc/$service" "${port}:8200" 2>$null
    } -ArgumentList $Ns, $svc, $LabPort
    Start-Sleep -Seconds 3
    return $job
}

function Stop-TuckPortForward($job) {
    if ($job) { $job | Stop-Job -PassThru | Remove-Job -Force -ErrorAction SilentlyContinue }
}

function Invoke-Tuck {
    param([string]$Method, [string]$Path, [string]$Token, [string]$Body = $null)
    $url = "http://127.0.0.1:${LabPort}$Path"
    $args = @("-s", "-w", "`n%{http_code}", "-X", $Method, $url)
    if ($Token) { $args += @("-H", "X-Tuck-Token: $Token") }
    if ($null -ne $Body -and $Method -ne "GET") {
        if ($Body -notmatch '^\{') { $args += @("--data-raw", $Body) }
        else { $args += @("-H", "Content-Type: application/json", "-d", $Body) }
    }
    $raw = & curl.exe @args
    $parts = $raw -split "`n"
    return @{ Code = $parts[-1]; Body = ($parts[0..([Math]::Max(0, $parts.Length - 2))] -join "`n") }
}

function Get-RootToken {
    if (Test-Path $TokenFile) {
        return (Get-Content $TokenFile -Raw).Trim()
    }
    $logs = kubectl -n $Ns logs deployment/tuck-server --tail=300 2>$null
    if (-not $logs) { $logs = kubectl -n $Ns logs statefulset/tuck-server --tail=300 2>$null }
    if ($logs -match 'ROOT TOKEN[^\r\n]*[\r\n]+\s+(tuck_[a-zA-Z0-9_-]+)') {
        $tok = $Matches[1]
    } elseif ($logs -match '\s+(tuck_[a-zA-Z0-9_-]{20,})') {
        $tok = $Matches[1]
    } else {
        throw "ROOT TOKEN not found in logs. If DB already exists, save token to: $TokenFile"
    }
    New-Item -ItemType Directory -Force -Path (Split-Path $TokenFile) | Out-Null
    Set-Content -Path $TokenFile -Value $tok -NoNewline
    Write-Host "Saved root token -> $TokenFile"
    return $tok
}

function Invoke-SmokeDemo {
    $token = (Get-Content $TokenFile -Raw).Trim()
    $pf = Start-TuckPortForward
    try {
        $h = Invoke-Tuck "GET" "/v1/health" ""
        if ($h.Code -ne "200") { throw "Health check failed ($($h.Code)). Is port-forward running?" }

        $demoVal = "minikube-lab-$(Get-Date -Format 'yyyyMMdd-HHmm')"
        $put = Invoke-Tuck "PUT" "/v1/secret/demo-app/api-key" $token $demoVal
        if ($put.Code -ne "204") { throw "PUT secret: $($put.Code) $($put.Body)" }

        kubectl apply -f deploy/examples/demo-tucksecret.yaml | Out-Null
        Write-Host "Waiting 40s for operator sync..."
        Start-Sleep -Seconds 40

        $b64 = kubectl get secret demo-app-credentials -o jsonpath='{.data.api-key}' 2>$null
        if (-not $b64) { throw "K8s Secret demo-app-credentials missing" }
        $decoded = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($b64))
        if ($decoded -ne $demoVal) { throw "Sync mismatch: '$decoded' != '$demoVal'" }
        Write-Host "OK: Tuck -> TuckSecret -> K8s Secret ($decoded)" -ForegroundColor Green
    } finally {
        Stop-TuckPortForward $pf
    }
}

if ($SmokeOnly) {
    Write-Step "Smoke test"
    if (-not (Test-Path $TokenFile)) { throw "Run full setup first (no $TokenFile)" }
    Invoke-SmokeDemo
    exit 0
}

# ── Full setup ──
Write-Step "Tools"
foreach ($cmd in @("minikube", "kubectl", "docker")) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "Missing: $cmd" }
}

$helmCmd = Get-Command helm -ErrorAction SilentlyContinue
if ($UseHelm -and -not $helmCmd) {
    Write-Host "Installing helm via winget..." -ForegroundColor Yellow
    winget install --id Helm.Helm -e --accept-package-agreements --accept-source-agreements
    $helmCmd = Get-Command helm -ErrorAction SilentlyContinue
    if (-not $helmCmd) { throw "helm not found. Install: winget install Helm.Helm" }
}

Write-Step "Minikube cluster"
$st = minikube status 2>&1 | Out-String
if ($st -match "host:\s*Stopped" -or $st -match "host:\s*Does not exist") {
    minikube start --driver=docker --cpus=2 --memory=4096
}
kubectl get nodes

if (-not $SkipBuild) {
    Write-Step "Docker build + load"
    docker build -f build/Dockerfile.server -t tuck-server:local .
    docker build -f build/Dockerfile.operator -t tuck-operator:local .
    minikube image load tuck-server:local
    minikube image load tuck-operator:local
    kubectl -n $Ns rollout restart deployment/tuck-server 2>$null
    kubectl -n $Ns rollout restart deployment/tuck-operator 2>$null
}

if ($ResetData) {
    Write-Step "Reset tuck data (new root token on next start)"
    kubectl -n $Ns scale deployment tuck-server --replicas=0 2>$null
    kubectl -n $Ns delete statefulset tuck-server --ignore-not-found
    kubectl -n $Ns delete pvc tuck-data --ignore-not-found --wait=true
    kubectl -n $Ns delete pvc data-tuck-server-0 --ignore-not-found --wait=true
    Remove-Item $TokenFile -Force -ErrorAction SilentlyContinue
    kubectl apply -f deploy/server/ | Out-Null
}

if ($UseHelm -and $helmCmd) {
    Write-Step "Helm install"
    $script:DeployMode = "helm"
    helm upgrade --install tuck deploy/helm/tuck -n $Ns --create-namespace `
        -f deploy/helm/tuck/values-minikube.yaml --wait --timeout 6m
    kubectl -n $Ns rollout status statefulset/tuck-server --timeout=180s
} else {
    Write-Step "kubectl deploy"
    $script:DeployMode = "kubectl"
    kubectl apply -f deploy/server/
    kubectl apply -f deploy/crd/
    kubectl apply -f deploy/operator/deployment.yaml
    kubectl apply -f deploy/operator/local.yaml
    kubectl -n $Ns rollout status deployment/tuck-server --timeout=180s
}

Write-Step "Root token"
$token = Get-RootToken

Write-Step "Operator K8s auth role"
$pf = Start-TuckPortForward
try {
    $body = '{"policies":["root"],"ttl":"24h"}'
    $r = Invoke-Tuck "PUT" "/v1/auth/kubernetes/role/${Ns}/tuck-operator" $token $body
    if ($r.Code -notmatch '^20') { Write-Warning "K8s role setup: $($r.Code) $($r.Body)" }
} finally {
    Stop-TuckPortForward $pf
}

Write-Step "Operator ready"
kubectl -n $Ns rollout status deployment/tuck-operator --timeout=120s

Write-Step "Smoke demo"
Invoke-SmokeDemo

$svc = if ($script:DeployMode -eq "helm") { "tuck-server" } else { "tuck" }
Write-Host @"

════════════════════════════════════════════════════════════
  Lab ready ($($script:DeployMode))
════════════════════════════════════════════════════════════

  Port-forward (отдельное окно, не закрывать):
    kubectl -n tuck port-forward svc/$svc ${LabPort}:8200

  Web UI (minikube tuck, не путать с локальным tuck на :8200):
    http://127.0.0.1:${LabPort}/ui/

  Token:
    $(Get-Content $TokenFile)

  Что попробовать в UI:
    1. Secrets -> Explorer -> Refresh (после пересборки tuck с фиксом list)
    2. Write Secret: path demo-app/api-key
    3. kubectl get secret demo-app-credentials  (проверка оператора)

  Повторить демо:
    .\scripts\minikube-lab.ps1 -SmokeOnly

"@ -ForegroundColor Green
