# Tuck automated test runner (Windows PowerShell 5.1+)
# Usage: .\scripts\run-all-tests.ps1
$ErrorActionPreference = "Continue"
$Results = @()
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

function Record($Id, $Name, $Pass, $Detail = "") {
    $script:Results += [PSCustomObject]@{ Id = $Id; Name = $Name; Pass = $Pass; Detail = $Detail }
    $mark = if ($Pass) { "PASS" } else { "FAIL" }
    Write-Host "[$mark] $Id - $Name"
    if ($Detail) { Write-Host "       $Detail" }
}

function Wait-Healthy($url, $timeoutSec = 15) {
    $deadline = (Get-Date).AddSeconds($timeoutSec)
    while ((Get-Date) -lt $deadline) {
        $code = & curl.exe -s -o NUL -w "%{http_code}" $url 2>$null
        if ($code -eq "200") { return $true }
        Start-Sleep -Milliseconds 500
    }
    return $false
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
    $f = Join-Path $env:TEMP ("tuck-test-" + [guid]::NewGuid().ToString() + ".json")
    [System.IO.File]::WriteAllText($f, $content, (New-Object System.Text.UTF8Encoding $false))
    return $f
}

function Start-PortForward {
    Get-NetTCPConnection -LocalPort 8200 -ErrorAction SilentlyContinue |
        ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 1
    Start-Process -FilePath "kubectl" -ArgumentList @("-n", "tuck", "port-forward", "svc/tuck", "8200:8200") -WindowStyle Hidden | Out-Null
    Start-Sleep -Seconds 3
}

# --- Unit tests ---
Write-Host "`n=== Unit tests ==="
go test ./... 2>&1 | Out-Null
Record "UNIT" "go test ./..." ($LASTEXITCODE -eq 0)
go test ./internal/seal/... -run TestShamir -v 2>&1 | Out-Null
Record "SHAMIR" "Shamir seal unit tests" ($LASTEXITCODE -eq 0)
go test ./internal/api/... -run TestSys -v 2>&1 | Out-Null
Record "SHAMIR-HTTP" "TestSysShamirUnseal integration" ($LASTEXITCODE -eq 0)

# --- Build ---
Write-Host "`n=== Build ==="
go build -o tuck.exe ./cmd/tuck
Record "BUILD" "go build tuck.exe" ($LASTEXITCODE -eq 0)

# --- Local server tests 1-7, 10 ---
Write-Host "`n=== Local server (tests 1-7, 10) ==="
$TestData = Join-Path $RepoRoot "testdata\local-run"
New-Item -ItemType Directory -Force -Path $TestData | Out-Null
$db = Join-Path $TestData "tuck.db"
$key = Join-Path $TestData "tuck-rootkey.bin"
Remove-Item $db, $key -ErrorAction SilentlyContinue

$TUCK_ADDR = "http://127.0.0.1:8201"
$tuckExe = Join-Path $RepoRoot "tuck.exe"
$logFile = Join-Path $TestData "tuck.log"

$tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @(
    "--addr=127.0.0.1:8201", "--data=$db", "--dev-seal-key=$key", "--k8s-api="
) -RedirectStandardError $logFile -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2

$log = Get-Content $logFile -Raw -ErrorAction SilentlyContinue
$TUCK_TOKEN = ""
if ($log -match '(tuck_[A-Za-z0-9_-]+)') { $TUCK_TOKEN = $Matches[1] }

try {
    Record "SETUP" "Extract root token" ([bool]$TUCK_TOKEN) $TUCK_TOKEN
    $ready = Wait-Healthy "$TUCK_ADDR/v1/health"
    Record "SETUP2" "Server ready" $ready

    $body = Curl-Body @("$TUCK_ADDR/v1/health")
    Record "1" "Health check" ($body -match '"sealed":false') $body

    $body = Curl-Body @("$TUCK_ADDR/v1/sys/seal-status")
    Record "2" "Seal status" ($body -match '"type":"dev"') $body

    $code = Curl-Status @("-X", "PUT", "$TUCK_ADDR/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "supersecret123")
    Record "3a" "Write secret" ($code -eq 204) "status $code"
    $body = Curl-Body @("$TUCK_ADDR/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3b" "Read secret" ($body -match "supersecret123") $body
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/db/nonexistent", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3c" "404 nonexistent" ($code -eq 404) "status $code"
    $code = Curl-Status @("-X", "DELETE", "$TUCK_ADDR/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3d" "Delete secret" ($code -eq 204) "status $code"
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/db/password", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "3e" "Deleted returns 404" ($code -eq 404) "status $code"

    $code = Curl-Status @("$TUCK_ADDR/v1/secret/anything")
    Record "4a" "No token 401" ($code -eq 401) "status $code"
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/anything", "-H", "X-Tuck-Token: tuck_invalid")
    Record "4b" "Bad token 401" ($code -eq 401) "status $code"

    $policyFile = Write-JsonBody '{"rules":[{"path":"secret/prod/*","capabilities":["read"]}]}'
    $code = Curl-Status @("-X", "PUT", "$TUCK_ADDR/v1/policy/prod-readonly", "-H", "X-Tuck-Token: $TUCK_TOKEN", "-H", "Content-Type: application/json", "-d", "@$policyFile")
    Record "5a" "Create policy" ($code -eq 204) "status $code"
    $body = Curl-Body @("$TUCK_ADDR/v1/policy/prod-readonly", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "5b" "Read policy" ($body -match "prod-readonly") $body
    $tokFile = Write-JsonBody '{"display_name":"prod-reader","policies":["prod-readonly"]}'
    $body = Curl-Body @("-X", "POST", "$TUCK_ADDR/v1/auth/token", "-H", "X-Tuck-Token: $TUCK_TOKEN", "-H", "Content-Type: application/json", "-d", "@$tokFile")
    $limited = ""
    if ($body) { try { $limited = ($body | ConvertFrom-Json).id } catch {} }
    Record "5c" "Create limited token" ([bool]$limited) $limited
    Curl-Status @("-X", "PUT", "$TUCK_ADDR/v1/secret/prod/api-key", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "prod-api-key-value") | Out-Null
    $body = Curl-Body @("$TUCK_ADDR/v1/secret/prod/api-key", "-H", "X-Tuck-Token: $limited")
    Record "5d" "Limited can read prod" ($body -match "prod-api-key-value") $body
    $code = Curl-Status @("-X", "PUT", "$TUCK_ADDR/v1/secret/prod/new-key", "-H", "X-Tuck-Token: $limited", "--data-raw", "val")
    Record "5e" "Limited cannot write" ($code -eq 403) "status $code"
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/staging/key", "-H", "X-Tuck-Token: $limited")
    Record "5f" "Limited cannot read staging" ($code -eq 403) "status $code"

    $tempFile = Write-JsonBody '{"display_name":"temp-token","policies":[],"ttl":"1h"}'
    $body = Curl-Body @("-X", "POST", "$TUCK_ADDR/v1/auth/token", "-H", "X-Tuck-Token: $TUCK_TOKEN", "-H", "Content-Type: application/json", "-d", "@$tempFile")
    $tempId = ""
    if ($body) { try { $tempId = ($body | ConvertFrom-Json).id } catch {} }
    Record "6a" "Create token with TTL" ([bool]$tempId) $tempId
    $code = Curl-Status @("$TUCK_ADDR/v1/auth/token/$tempId", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "6b" "Get token" ($code -eq 200) "status $code"
    $code = Curl-Status @("-X", "DELETE", "$TUCK_ADDR/v1/auth/token/$tempId", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "6c" "Revoke token" ($code -eq 204) "status $code"
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/anything", "-H", "X-Tuck-Token: $tempId")
    Record "6d" "Revoked token 401" ($code -eq 401) "status $code"

    $code = Curl-Status @("-X", "POST", "$TUCK_ADDR/v1/sys/seal", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "7a" "Manual seal" ($code -eq 200) "status $code"
    $body = Curl-Body @("$TUCK_ADDR/v1/sys/seal-status")
    Record "7b" "Sealed status" ($body -match '"sealed":true') $body
    $code = Curl-Status @("$TUCK_ADDR/v1/secret/anything", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "7c" "Sealed 503" ($code -eq 503) "status $code"
    $code = Curl-Status @("-X", "POST", "$TUCK_ADDR/v1/sys/unseal", "-H", "Content-Type: application/json", "--data-raw", '{"key":"anything"}')
    Record "7d" "Dev unseal API 400" ($code -eq 400) "status $code"

    Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    $tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @("--addr=127.0.0.1:8201", "--data=$db", "--dev-seal-key=$key", "--k8s-api=") -PassThru -WindowStyle Hidden
    Wait-Healthy "$TUCK_ADDR/v1/health" | Out-Null
    $body = Curl-Body @("$TUCK_ADDR/v1/health")
    Record "7e" "Restart auto-unseal" ($body -match '"sealed":false') $body

    Curl-Status @("-X", "PUT", "$TUCK_ADDR/v1/secret/persist-test", "-H", "X-Tuck-Token: $TUCK_TOKEN", "--data-raw", "should-survive-restart") | Out-Null
    Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    $log3 = Join-Path $TestData "tuck3.log"
    $tuckProc = Start-Process -FilePath $tuckExe -ArgumentList @("--addr=127.0.0.1:8201", "--data=$db", "--dev-seal-key=$key", "--k8s-api=") -RedirectStandardError $log3 -PassThru -WindowStyle Hidden
    Wait-Healthy "$TUCK_ADDR/v1/health" | Out-Null
    $log3txt = Get-Content $log3 -Raw -ErrorAction SilentlyContinue
    Record "10a" "No new root token on restart" (-not ($log3txt -match "ROOT TOKEN"))
    $body = Curl-Body @("$TUCK_ADDR/v1/secret/persist-test", "-H", "X-Tuck-Token: $TUCK_TOKEN")
    Record "10b" "Persist after restart" ($body -match "should-survive-restart") $body
} finally {
    Stop-Process -Id $tuckProc.Id -Force -ErrorAction SilentlyContinue
}

# --- K8s tests 8-9, 10k (pod restart) ---
Write-Host "`n=== Minikube tests 8-9, 10k ==="
$k8sAddr = "http://127.0.0.1:8200"
Start-PortForward
$k8sUp = Wait-Healthy "$k8sAddr/v1/health" 20

if (-not $k8sUp) {
    Record "8" "K8s SA auth" $false "port-forward not on :8200"
    Record "9" "TuckSecret operator" $false "port-forward not on :8200"
    Record "10k" "K8s pod restart persist" $false "port-forward not on :8200"
} else {
    $tokenFile = Join-Path $RepoRoot "testdata\minikube-root-token.txt"
    $k8sLogs = kubectl -n tuck logs deployment/tuck-server 2>&1 | Out-String
    $K8S_TOKEN = ""
    if ($k8sLogs -match '(tuck_[A-Za-z0-9_-]+)') { $K8S_TOKEN = $Matches[1] }
    if (-not $K8S_TOKEN -and (Test-Path $tokenFile)) { $K8S_TOKEN = (Get-Content $tokenFile -Raw).Trim() }
    if ($K8S_TOKEN) { Set-Content -Path $tokenFile -Value $K8S_TOKEN -Encoding UTF8 -NoNewline }
    Record "K8S-TOKEN" "K8s root token available" ([bool]$K8S_TOKEN) $(if ($K8S_TOKEN) { $K8S_TOKEN.Substring(0, [Math]::Min(20, $K8S_TOKEN.Length)) + "..." } else { "save from: kubectl -n tuck logs deployment/tuck-server" })
    kubectl create serviceaccount test-app -n default --dry-run=client -o yaml 2>$null | kubectl apply -f - 2>&1 | Out-Null
    $SA_TOKEN = kubectl create token test-app --duration=1h 2>&1
    $appPolicyFile = Write-JsonBody '{"rules":[{"path":"secret/app/*","capabilities":["read"]}]}'
    $roleFile = Write-JsonBody '{"policies":["app-policy"],"ttl":"1h"}'
    Curl-Status @("-X", "PUT", "$k8sAddr/v1/policy/app-policy", "-H", "X-Tuck-Token: $K8S_TOKEN", "-H", "Content-Type: application/json", "-d", "@$appPolicyFile") | Out-Null
    Curl-Status @("-X", "PUT", "$k8sAddr/v1/auth/kubernetes/role/default/test-app", "-H", "X-Tuck-Token: $K8S_TOKEN", "-H", "Content-Type: application/json", "-d", "@$roleFile") | Out-Null
    $loginFile = Write-JsonBody ((@{ token = $SA_TOKEN } | ConvertTo-Json -Compress))
    $lrBody = Curl-Body @("-X", "POST", "$k8sAddr/v1/auth/kubernetes/login", "-H", "Content-Type: application/json", "-d", "@$loginFile")
    $appTok = ""
    if ($lrBody) { try { $appTok = ($lrBody | ConvertFrom-Json).token } catch {} }
    Curl-Status @("-X", "PUT", "$k8sAddr/v1/secret/app/config", "-H", "X-Tuck-Token: $K8S_TOKEN", "--data-raw", "app-config-value") | Out-Null
    $arBody = Curl-Body @("$k8sAddr/v1/secret/app/config", "-H", "X-Tuck-Token: $appTok")
    Record "8" "K8s SA auth" ($appTok -and ($arBody -match "app-config-value")) $(if ($appTok) { $arBody } else { $lrBody })

    $secVal = kubectl get secret myapp-db-credentials -n default -o jsonpath='{.data.password}' 2>&1
    if ($secVal -and $secVal -notmatch "Error") {
        $decoded = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($secVal))
        Record "9a" "TuckSecret sync" ($decoded.Length -gt 0) "value=$decoded"
    } else {
        Record "9a" "TuckSecret sync" $false "secret not found"
    }
    kubectl get tucksecret db-password -n default 2>&1 | Out-Null
    Record "9b" "TuckSecret CRD" ($LASTEXITCODE -eq 0)

    Curl-Status @("-X", "PUT", "$k8sAddr/v1/secret/k8s-persist-test", "-H", "X-Tuck-Token: $K8S_TOKEN", "--data-raw", "survives-pod-restart") | Out-Null
    kubectl -n tuck rollout restart deployment/tuck-server 2>&1 | Out-Null
    kubectl -n tuck rollout status deployment/tuck-server --timeout=120s 2>&1 | Out-Null
    Start-PortForward
    Wait-Healthy "$k8sAddr/v1/health" 60 | Out-Null
    $body = Curl-Body @("$k8sAddr/v1/health")
    $pBody = Curl-Body @("$k8sAddr/v1/secret/k8s-persist-test", "-H", "X-Tuck-Token: $K8S_TOKEN")
    Record "10k" "K8s pod restart persist" ($body -match '"sealed":false' -and $pBody -match "survives-pod-restart") $pBody
}

# --- Write results ---
Write-Host "`n=== Summary ==="
$passed = @($Results | Where-Object { $_.Pass }).Count
$failed = @($Results | Where-Object { -not $_.Pass }).Count
Write-Host "Passed: $passed  Failed: $failed  Total: $($Results.Count)"

$outFile = Join-Path $RepoRoot "docs\TEST_RESULTS.md"
$date = Get-Date -Format "yyyy-MM-dd HH:mm"
$goVer = (go version) -replace '.* (go[\d.]+).*', '$1'
$mkVer = (minikube version --short 2>$null)

$lines = @(
    "# Tuck - test results",
    "",
    "**Run date:** $date",
    "**Environment:** Windows, $goVer, minikube $mkVer",
    "",
    "| ID | Test | Result | Details |",
    "|----|------|--------|---------|"
)
foreach ($r in $Results) {
    $status = if ($r.Pass) { "PASS" } else { "FAIL" }
    $detail = [string]$r.Detail
    $detail = $detail -replace '\|', '/'
    if ($detail.Length -gt 80) { $detail = $detail.Substring(0, 77) + "..." }
    $lines += "| $($r.Id) | $($r.Name) | $status | $detail |"
}
$lines += ""
$lines += "## Summary"
$lines += ""
$lines += "- Passed: $passed / $($Results.Count)"
$lines += "- Failed: $failed"
$lines += ""
$lines += "Scenarios: [TESTING.md](TESTING.md)"
Set-Content -Path $outFile -Value ($lines -join "`n") -Encoding UTF8
Write-Host "Results: docs/TEST_RESULTS.md"
if ($failed -gt 0) { exit 1 }
