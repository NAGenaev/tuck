# E2E demo: Tuck → operator → K8s Secret

Quick hands-on check that the full stack works (~2 min).

## Prerequisites

```powershell
# Terminal 1
kubectl -n tuck port-forward svc/tuck 8200:8200

# Terminal 2
$TUCK_ADDR  = "http://127.0.0.1:8200"
$TUCK_TOKEN = (Get-Content testdata\minikube-root-token.txt).Trim()
```

## Steps

```powershell
# 1. Write secret to Tuck
curl.exe -s -X PUT "$TUCK_ADDR/v1/secret/demo-app/api-key" `
  -H "X-Tuck-Token: $TUCK_TOKEN" --data-raw "super-secret-demo-2026"

# 2. Apply TuckSecret
kubectl apply -f deploy/examples/demo-tucksecret.yaml

# 3. Wait for sync
Start-Sleep -Seconds 35

# 4. Verify K8s Secret
kubectl get secret demo-app-credentials -o jsonpath='{.data.api-key}' | `
  ForEach-Object { [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($_)) }
# => super-secret-demo-2026
```

See also [MINIKUBE.md](MINIKUBE.md) and [TESTING.md](TESTING.md).
