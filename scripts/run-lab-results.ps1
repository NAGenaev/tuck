# Full lab test run — results in lab-results/runs/<timestamp>/
# Usage: .\scripts\run-lab-results.ps1 [-SkipUnit] [-ReloadK8sImage]

param(
    [switch]$SkipUnit,
    [switch]$ReloadK8sImage
)
$ErrorActionPreference = "Continue"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

$RunId = Get-Date -Format "yyyy-MM-dd_HHmmss"
$OutDir = Join-Path $RepoRoot "lab-results\runs\$RunId"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$Results = [System.Collections.Generic.List[object]]::new()
$LocalPort = 8202
$K8sPort = 8201
$LocalAddr = "http://127.0.0.1:$LocalPort"
$K8sAddr = "http://127.0.0.1:$K8sPort"

function Record($Id, $Name, $Pass, $Detail = "", $Env = "") {
    $o = [PSCustomObject]@{ Id = $Id; Name = $Name; Pass = $Pass; Detail = $Detail; Environment = $Env }
    $script:Results.Add($o)
    $mark = if ($Pass) { "PASS" } else { "FAIL" }
    Write-Host "[$mark] $Id - $Name $(if ($Env) { "($Env)" })"
    if ($Detail) { Write-Host "       $Detail" }
}

function Curl-Status([string[]]$CurlArg) {
    $out = & curl.exe -s -o NUL -w "%{http_code}" @CurlArg 2>$null
    if ($out -match '^\d{3}$') { return [int]$out }
    return 0
}

function Curl-Body([string[]]$CurlArg) {
    return (& curl.exe -s @CurlArg 2>$null)
}

function Write-JsonBody($content) {
    $f = Join-Path $env:TEMP ("tuck-lab-" + [guid]::NewGuid().ToString() + ".json")
    [System.IO.File]::WriteAllText($f, $content, (New-Object System.Text.UTF8Encoding $false))
    return $f
}

function Wait-Healthy($url, $timeoutSec = 20) {
    $deadline = (Get-Date).AddSeconds($timeoutSec)
    while ((Get-Date) -lt $deadline) {
        if ((Curl-Status @($url)) -eq 200) { return $true }
        Start-Sleep -Milliseconds 400
    }
    return $false
}

function Stop-K8sPortForward {
    Get-NetTCPConnection -LocalPort $K8sPort -State Listen -ErrorAction SilentlyContinue |
        ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Milliseconds 500
}

function Ensure-K8sPortForward {
    if ((Curl-Status @("$K8sAddr/v1/health")) -eq 200) { return $true }
    Stop-K8sPortForward
    Start-Process -FilePath "kubectl" -ArgumentList @("-n", "tuck", "port-forward", "svc/tuck", "${K8sPort}:8200") -WindowStyle Hidden | Out-Null
    return (Wait-Healthy "$K8sAddr/v1/health" 30)
}

# ── UNIT ──
Write-Host "`n=== Unit tests ===" -ForegroundColor Cyan
$unitLog = Join-Path $OutDir "go-test.log"
if ($SkipUnit) {
    Record "UNIT" "go test ./... (skipped)" $true "use -SkipUnit to bypass" "go"
} else {
    go test ./... -count=1 -p 1 2>&1 | Tee-Object -FilePath $unitLog | Out-Null
    $unitFail = $LASTEXITCODE -ne 0
    Record "UNIT" "go test ./..." (-not $unitFail) "exit=$LASTEXITCODE; see go-test.log" "go"
    go test ./internal/api/... -run "TestSecretListGETCompat|TestKVRoundTrip" -count=1 2>&1 | Out-Null
    Record "UNIT-LIST" "API list=true compat tests" ($LASTEXITCODE -eq 0) "" "go"
}

if ($ReloadK8sImage) {
    Write-Host "`n=== Reload K8s image ===" -ForegroundColor Cyan
    docker build -f build/Dockerfile.server -t tuck-server:local . 2>&1 | Out-Null
    minikube ssh "docker rmi -f tuck-server:local" 2>&1 | Out-Null
    minikube image load tuck-server:local 2>&1 | Out-Null
    kubectl -n tuck rollout restart deployment/tuck-server 2>&1 | Out-Null
    kubectl -n tuck rollout status deployment/tuck-server --timeout=120s 2>&1 | Out-Null
    Stop-K8sPortForward
    Record "K8S-IMG" "Reload tuck-server:local" ($LASTEXITCODE -eq 0) "" "minikube"
}

function Get-K8sRootToken {
    $tokenFile = Join-Path $RepoRoot "testdata\minikube-root-token.txt"
    $tok = ""
    if (Test-Path $tokenFile) { $tok = (Get-Content $tokenFile -Raw).Trim() }
    if ($tok) {
        $code = Curl-Status @("$K8sAddr/v1/sys/seal-status", "-H", "X-Tuck-Token: $tok")
        if ($code -eq 200) { return $tok }
    }
    $logs = kubectl -n tuck logs deployment/tuck-server --tail=200 2>$null
    if ($logs -match 'ROOT TOKEN[^\r\n]*[\r\n]+\s+(tuck_[a-zA-Z0-9_-]+)') {
        $tok = $Matches[1]
    } elseif ($logs -match '\s+(tuck_[a-zA-Z0-9_-]{20,})') {
        $tok = $Matches[1]
    }
    if ($tok) {
        New-Item -ItemType Directory -Force -Path (Split-Path $tokenFile) | Out-Null
        Set-Content -Path $tokenFile -Value $tok -NoNewline -Encoding ascii
    }
    return $tok
}

# ── BUILD ──
Write-Host "`n=== Build ===" -ForegroundColor Cyan
go build -o tuck.exe ./cmd/tuck 2>&1 | Out-Null
Record "BUILD" "go build tuck.exe" ($LASTEXITCODE -eq 0) "" "build"

# ── LOCAL server (isolated port 8202) ──
Write-Host "`n=== Local API tests 1-7, 10, UI-list ===" -ForegroundColor Cyan
$TestData = Join-Path $RepoRoot "testdata\lab-run-$RunId"
New-Item -ItemType Directory -Force -Path $TestData | Out-Null
$db = Join-Path $TestData "tuck.db"
$key = Join-Path $TestData "tuck-rootkey.bin"
$logFile = Join-Path $TestData "tuck.log"
$tuckExe = Join-Path $RepoRoot "tuck.exe"

$tuckProc = $null
try {
    $tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @(
        "--addr=127.0.0.1:$LocalPort", "--data=$db", "--dev-seal-key=$key", "--k8s-api="
    ) -RedirectStandardError $logFile -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 2
    $log = Get-Content $logFile -Raw -ErrorAction SilentlyContinue
    $TUCK_TOKEN = ""
    if ($log -match '(tuck_[A-Za-z0-9_-]+)') { $TUCK_TOKEN = $Matches[1] }

    Record "L-SETUP" "Local server + root token" ([bool]$TUCK_TOKEN) $TUCK_TOKEN "local:$LocalPort"
    $ready = Wait-Healthy "$LocalAddr/v1/health"
    Record "L-READY" "Health endpoint" $ready "" "local"

    $body = Curl-Body @("$LocalAddr/v1/health")
    Record "1" "Health sealed=false" ($body -match '"sealed":false') $body "local"
    $body = Curl-Body @("$LocalAddr/v1/sys/seal-status")
    Record "2" "Seal status dev" ($body -match '"type":"dev"') $body "local"

    $code = Curl-Status @("-X", "PUT", "$LocalAddr/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "supersecret123")
    Record "3a" "KV put" ($code -eq 204) "status $code" "local"
    $body = Curl-Body @("$LocalAddr/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3b" "KV get" ($body -match "supersecret123") $body "local"
    $code = Curl-Status @("$LocalAddr/v1/secret/db/missing", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3c" "KV 404" ($code -eq 404) "status $code" "local"
    $code = Curl-Status @("-X", "DELETE", "$LocalAddr/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3d" "KV delete" ($code -eq 204) "status $code" "local"

    $code = Curl-Status @("$LocalAddr/v1/secret/anything")
    Record "4a" "Auth no token" ($code -eq 401) "status $code" "local"
    $code = Curl-Status @("$LocalAddr/v1/secret/anything", "-H", "X-Tuck-Token: tuck_invalid")
    Record "4b" "Auth bad token" ($code -eq 401) "status $code" "local"

    $pf = Write-JsonBody '{"rules":[{"path":"secret/prod/*","capabilities":["read"]}]}'
    Curl-Status @("-X", "PUT", "$LocalAddr/v1/policy/prod-readonly", "-H", "X-Tuck-Token: $TUCK_TOKEN", "-H", "Content-Type: application/json", "-d", "@$pf") | Out-Null
    $body = Curl-Body @("$LocalAddr/v1/policy/prod-readonly", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "5" "Policy ACL" ($body -match "prod-readonly") $body "local"

    $tf = Write-JsonBody '{"display_name":"temp","policies":[],"ttl":"1h"}'
    $body = Curl-Body @("-X", "POST", "$LocalAddr/v1/auth/token", "-H", "X-Tuck-Token: $TUCK_TOKEN", "-H", "Content-Type: application/json", "-d", "@$tf")
    $tid = ""; if ($body) { try { $tid = ($body | ConvertFrom-Json).id } catch {} }
    Record "6" "Token create" ([bool]$tid) $tid "local"

    $code = Curl-Status @("-X", "POST", "$LocalAddr/v1/sys/seal", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "7a" "Manual seal" ($code -eq 200) "status $code" "local"
    $code = Curl-Status @("$LocalAddr/v1/secret/x", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "7b" "Sealed 503" ($code -eq 503) "status $code" "local"

    Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    $tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @("--addr=127.0.0.1:$LocalPort", "--data=$db", "--dev-seal-key=$key", "--k8s-api=") -PassThru -WindowStyle Hidden
    Wait-Healthy "$LocalAddr/v1/health" | Out-Null
    Curl-Status @("-X", "PUT", "$LocalAddr/v1/secret/persist-test", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "persist-ok") | Out-Null
    Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    $tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @("--addr=127.0.0.1:$LocalPort", "--data=$db", "--dev-seal-key=$key", "--k8s-api=") -PassThru -WindowStyle Hidden
    Wait-Healthy "$LocalAddr/v1/health" | Out-Null
    $body = Curl-Body @("$LocalAddr/v1/secret/persist-test", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "10" "Persist restart" ($body -match "persist-ok") $body "local"

    # UI Explorer API (browser-safe list)
    Curl-Status @("-X", "PUT", "$LocalAddr/v1/secret/app/one", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "v1") | Out-Null
    $body = Curl-Body @("$LocalAddr/v1/secret/?list=true", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "UI-LIST" "GET ?list=true Explorer" ($body -match '"keys"') $body "local"
    $code = Curl-Status @("-X", "PUT", "$LocalAddr/v2/secret/ui-test", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "v2data")
    Record "UI-KV2" "KV v2 write" ($code -eq 200) "status $code" "local"
    $body = Curl-Body @("$LocalAddr/v2/secret/metadata/?list=true", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "UI-KV2-LIST" "KV v2 metadata list" ($body -match 'ui-test') $body "local"

} finally {
    if ($tuckProc) { Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue }
}

# ── MINIKUBE ──
Write-Host "`n=== Minikube tests 8-9, operator, cluster ===" -ForegroundColor Cyan
$k8sUp = Ensure-K8sPortForward
Record "K8S-PF" "Port-forward :$K8sPort" $k8sUp "" "minikube"

if ($k8sUp) {
    $tokenFile = Join-Path $RepoRoot "testdata\minikube-root-token.txt"
    $K8S_TOKEN = Get-K8sRootToken
    Record "K8S-TOKEN" "Root token file" ([bool]$K8S_TOKEN) $(if ($K8S_TOKEN) { $K8S_TOKEN.Substring(0,20)+"..." } else { "missing" }) "minikube"

    $body = Curl-Body @("$K8sAddr/v1/health")
    Record "K8S-1" "Health" ($body -match '"sealed":false') $body "minikube"

    if ($K8S_TOKEN) {
        kubectl create serviceaccount test-app -n default --dry-run=client -o yaml 2>$null | kubectl apply -f - 2>&1 | Out-Null
        $SA_TOKEN = kubectl create token test-app --duration=1h 2>&1
        $ap = Write-JsonBody '{"rules":[{"path":"secret/app/*","capabilities":["read"]}]}'
        $rl = Write-JsonBody '{"policies":["app-policy"],"ttl":"1h"}'
        Curl-Status @("-X", "PUT", "$K8sAddr/v1/policy/app-policy", "-H", "X-Tuck-Token: $K8S_TOKEN", "-H", "Content-Type: application/json", "-d", "@$ap") | Out-Null
        Curl-Status @("-X", "PUT", "$K8sAddr/v1/auth/kubernetes/role/default/test-app", "-H", "X-Tuck-Token: $K8S_TOKEN", "-H", "Content-Type: application/json", "-d", "@$rl") | Out-Null
        $lf = Write-JsonBody ((@{ token = $SA_TOKEN } | ConvertTo-Json -Compress))
        $lrBody = Curl-Body @("-X", "POST", "$K8sAddr/v1/auth/kubernetes/login", "-H", "Content-Type: application/json", "-d", "@$lf")
        $appTok = ""
        if ($lrBody) {
            try {
                $lr = $lrBody | ConvertFrom-Json
                if ($lr.token) { $appTok = $lr.token }
                elseif ($lr.auth.client_token) { $appTok = $lr.auth.client_token }
            } catch {}
        }
        Curl-Status @("-X", "PUT", "$K8sAddr/v1/secret/app/config", "-H", "X-Tuck-Token: $K8S_TOKEN", "--data-raw", "app-config-value") | Out-Null
        $arBody = Curl-Body @("$K8sAddr/v1/secret/app/config", "-H", "X-Tuck-Token: $appTok")
        Record "8" "K8s SA auth login+read" ($appTok -and ($arBody -match "app-config-value")) $(if ($appTok) { "ok" } else { $lrBody }) "minikube"

        $opRole = Write-JsonBody '{"policies":["root"],"ttl":"24h"}'
        Curl-Status @("-X", "PUT", "$K8sAddr/v1/auth/kubernetes/role/tuck/tuck-operator", "-H", "X-Tuck-Token: $K8S_TOKEN", "-H", "Content-Type: application/json", "-d", "@$opRole") | Out-Null
        kubectl -n tuck rollout restart deployment/tuck-operator 2>&1 | Out-Null
        kubectl -n tuck rollout status deployment/tuck-operator --timeout=90s 2>&1 | Out-Null

        $demoVal = "lab-run-$RunId"
        $code = Curl-Status @("-X", "PUT", "$K8sAddr/v1/secret/demo-app/api-key", "-H", "X-Tuck-Token: $K8S_TOKEN", "--data-raw", $demoVal)
        Record "9a" "PUT secret for operator" ($code -eq 204) "status $code" "minikube"
        kubectl delete tucksecret demo-api-key -n default --ignore-not-found 2>&1 | Out-Null
        kubectl delete secret demo-app-credentials -n default --ignore-not-found 2>&1 | Out-Null
        Start-Sleep -Seconds 2
        kubectl apply -f deploy/examples/demo-tucksecret.yaml 2>&1 | Out-Null
        Start-Sleep -Seconds 45
        $b64 = kubectl get secret demo-app-credentials -o jsonpath='{.data.api-key}' 2>$null
        $syncOk = $false; $decoded = ""
        if ($b64) {
            $decoded = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($b64))
            $syncOk = ($decoded -eq $demoVal)
        }
        Record "9b" "TuckSecret operator sync" $syncOk "got=$decoded want=$demoVal" "minikube"
        kubectl get tucksecret demo-api-key 2>&1 | Out-Null
        Record "9c" "TuckSecret CRD exists" ($LASTEXITCODE -eq 0) "" "minikube"

        $body = Curl-Body @("$K8sAddr/v1/secret/?list=true", "-H", "X-Tuck-Token: $K8S_TOKEN")
        $listOk = $body -match '"keys"'
        Record "UI-K8S-LIST" "K8s UI list=true" $listOk $body "minikube"

        kubectl -n tuck get pods -l app=tuck-server -o jsonpath='{.items[0].status.phase}' 2>$null | Out-Null
        Record "K8S-PODS" "tuck-server pod Running" ($LASTEXITCODE -eq 0 -and (kubectl -n tuck get pods -l app=tuck-server -o jsonpath='{.items[0].status.phase}' 2>$null) -eq "Running") "" "minikube"
        kubectl -n tuck get pods -l app=tuck-operator -o jsonpath='{.items[0].status.phase}' 2>$null | Out-Null
        Record "K8S-OP" "tuck-operator Running" ((kubectl -n tuck get pods -l app=tuck-operator -o jsonpath='{.items[0].status.phase}' 2>$null) -eq "Running") "" "minikube"
    }
} else {
    Record "8" "K8s SA auth" $false "no port-forward" "minikube"
    Record "9b" "TuckSecret sync" $false "no port-forward" "minikube"
}

# ── CLUSTER sanity ──
Write-Host "`n=== Cluster sanity ===" -ForegroundColor Cyan
$nodes = kubectl get nodes -o json 2>$null | ConvertFrom-Json
$nodeReady = $false
if ($nodes -and $nodes.items) {
    $cond = $nodes.items[0].status.conditions | Where-Object { $_.type -eq "Ready" } | Select-Object -First 1
    if ($cond) { $nodeReady = ($cond.status -eq "True") }
}
Record "CLUSTER" "minikube node Ready" $nodeReady $(if ($cond) { $cond.status } else { "no data" }) "minikube"
$crd = kubectl get crd tucksecrets.tuck.io 2>&1
Record "CRD" "TuckSecret CRD installed" ($LASTEXITCODE -eq 0) "" "minikube"

# ── Write reports ──
$passed = @($Results | Where-Object { $_.Pass }).Count
$failed = @($Results | Where-Object { -not $_.Pass }).Count
$date = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

$md = @(
    "# Lab test run $RunId",
    "",
    "**Date:** $date",
    "**Host:** Windows",
    "**Go:** $(go version)",
    "",
    "## Summary",
    "",
    "| Metric | Value |",
    "|--------|-------|",
    "| Passed | $passed |",
    "| Failed | $failed |",
    "| Total | $($Results.Count) |",
    "",
    "## Results",
    "",
    "| ID | Test | Env | Result | Details |",
    "|----|------|-----|--------|---------|"
)
foreach ($r in $Results) {
    $st = if ($r.Pass) { "PASS" } else { "FAIL" }
    $d = [string]$r.Detail -replace '\|', '/' -replace "`n", " "
    if ($d.Length -gt 100) { $d = $d.Substring(0, 97) + "..." }
    $md += "| $($r.Id) | $($r.Name) | $($r.Environment) | $st | $d |"
}
$md += ""
$md += "## Artifacts"
$md += ""
$md += "- go-test.log - full unit test output"
$md += "- results.json - machine-readable"
if ($failed -gt 0) {
    $failMd = @(
        "# Failures - run $RunId",
        "",
        "Razbor upavshih testov.",
        ""
    )
    foreach ($r in ($Results | Where-Object { -not $_.Pass })) {
        $failMd += "## $($r.Id) - $($r.Name)"
        $failMd += ""
        $failMd += "- **Env:** $($r.Environment)"
        $failMd += "- **Detail:** $($r.Detail)"
        $failMd += ""
    }
    $failMd | Set-Content -Path (Join-Path $OutDir "FAILURES.md") -Encoding UTF8
    $md += "- FAILURES.md - failed test details"
}
$md | Set-Content -Path (Join-Path $OutDir "RESULTS.md") -Encoding UTF8

$Results | ConvertTo-Json -Depth 3 | Set-Content -Path (Join-Path $OutDir "results.json") -Encoding UTF8

# Update lab-results index
$indexPath = Join-Path $RepoRoot "lab-results\README.md"
$latest = @(
    "# Lab test results",
    "",
    "Автоматические прогоны сценариев Tuck (unit + local API + minikube).",
    "",
    "## Последний прогон",
    "",
    "- **Run:** [$RunId](runs/$RunId/RESULTS.md)",
    "- **Passed:** $passed / $($Results.Count)",
    "- **Failed:** $failed",
    "",
    "## Запуск",
    "",
    "```powershell",
    ".\scripts\run-lab-results.ps1",
    ".\scripts\run-lab-results.ps1 -SkipUnit          # быстрый прогон API",
    ".\scripts\run-lab-results.ps1 -ReloadK8sImage    # пересобрать образ в minikube",
    "```",
    "",
    "## История",
    "",
    "| Run | Passed | Failed |",
    "|-----|--------|--------|"
)
Get-ChildItem (Join-Path $RepoRoot "lab-results\runs") -Directory -ErrorAction SilentlyContinue |
    Sort-Object Name -Descending |
    ForEach-Object {
        $j = Join-Path $_.FullName "results.json"
        if (Test-Path $j) {
            $arr = Get-Content $j -Raw | ConvertFrom-Json
            if ($arr -isnot [array]) { $arr = @($arr) }
            $p = @($arr | Where-Object { $_.Pass }).Count
            $f = @($arr | Where-Object { -not $_.Pass }).Count
            $latest += "| [$($_.Name)](runs/$($_.Name)/RESULTS.md) | $p | $f |"
        }
    }
Set-Content -Path $indexPath -Value ($latest -join "`n") -Encoding UTF8

Write-Host "`n=== Done: $passed passed, $failed failed ===" -ForegroundColor $(if ($failed -eq 0) { "Green" } else { "Yellow" })
Write-Host "Results: lab-results\runs\$RunId\RESULTS.md"
if ($failed -gt 0) { exit 1 }
