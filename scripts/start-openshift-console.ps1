# Deploy and open the real OpenShift Console on minikube.
# Usage: .\scripts\start-openshift-console.ps1
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

Write-Host "=== OpenShift Console (origin-console) on minikube ===" -ForegroundColor Cyan

minikube status -f "{{.Host}}" 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "minikube is not running. Start it first: minikube start --driver=docker" -ForegroundColor Red
    exit 1
}

kubectl apply -f deploy/console/rbac.yaml

$token = kubectl create token console -n openshift-console --duration=8760h 2>&1
if ($LASTEXITCODE -ne 0) { throw "failed to create console token: $token" }

kubectl create secret generic console-token -n openshift-console `
    --from-literal=token=$token `
    --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f deploy/console/deployment.yaml

Write-Host "Waiting for console pod..."
kubectl -n openshift-console rollout status deployment/console --timeout=300s

# Free port 9000 if occupied
Get-NetTCPConnection -LocalPort 9000 -ErrorAction SilentlyContinue |
    ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 1

Write-Host ""
Write-Host "Starting port-forward to http://localhost:9000" -ForegroundColor Green
Write-Host "Keep this window open while using the console." -ForegroundColor Yellow
Write-Host ""
Write-Host "Open in browser: http://localhost:9000" -ForegroundColor Green
Write-Host "(Red Hat OpenShift branding — same UI as OpenShift, on minikube)" -ForegroundColor DarkGray
Write-Host ""

Start-Process "http://localhost:9000"
kubectl -n openshift-console port-forward svc/console 9000:9000
