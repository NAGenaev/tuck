# Tuck — локальное тестирование в minikube (Windows)

Пошаговый гайд для проверки k8s auth, CRD и оператора на Windows + Docker Desktop.

## Требования

- Docker Desktop (запущен)
- Go 1.22+ (для unit-тестов)
- kubectl
- minikube: `winget install Kubernetes.minikube`

## 1. Поднять кластер

```powershell
minikube start --driver=docker --cpus=2 --memory=4096
kubectl get nodes
```

## 2. Собрать образы

```powershell
cd D:\Projects\tuck

docker build -f build/Dockerfile.server -t tuck-server:local .
docker build -f build/Dockerfile.operator -t tuck-operator:local .

minikube image load tuck-server:local
minikube image load tuck-operator:local
```

## 3. Задеплоить Tuck-сервер

```powershell
kubectl apply -f deploy/server/
kubectl -n tuck rollout status deployment/tuck-server

# Root token — только при первом старте (в логах)
kubectl -n tuck logs deployment/tuck-server | Select-String "ROOT TOKEN" -Context 0,1
```

Сохраните root token:

```powershell
$TUCK_TOKEN = "tuck_..."   # из логов
```

## 4. Port-forward для curl с хоста

В отдельном окне PowerShell:

```powershell
kubectl -n tuck port-forward svc/tuck 8200:8200
```

В основном окне:

```powershell
$TUCK_ADDR = "http://127.0.0.1:8200"
curl $TUCK_ADDR/v1/health
```

## 5. Kubernetes ServiceAccount auth (Тест 8)

```powershell
kubectl create serviceaccount test-app -n default

$SA_TOKEN = kubectl create token test-app --duration=1h

curl -X PUT $TUCK_ADDR/v1/policy/app-policy `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"rules":[{"path":"secret/app/*","capabilities":["read"]}]}'

curl -X PUT "$TUCK_ADDR/v1/auth/kubernetes/role/default/test-app" `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["app-policy"],"ttl":"1h"}'

curl -X POST $TUCK_ADDR/v1/auth/kubernetes/login `
  -H "Content-Type: application/json" `
  -d "{`"token`":`"$SA_TOKEN`"}"
```

## 6. TuckSecret CRD + оператор (Тест 9)

```powershell
kubectl apply -f deploy/crd/
kubectl apply -f deploy/operator/deployment.yaml
kubectl apply -f deploy/operator/local.yaml

# Role binding для оператора (dev: root policy)
curl -X PUT "$TUCK_ADDR/v1/auth/kubernetes/role/tuck/tuck-operator" `
  -H "X-Tuck-Token: $TUCK_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"policies":["root"],"ttl":"5m"}'

kubectl -n tuck rollout status deployment/tuck-operator

curl -X PUT $TUCK_ADDR/v1/secret/myapp/db-password `
  -H "X-Tuck-Token: $TUCK_TOKEN" -d "postgres-secret-xyz"
```

Пример: `deploy/examples/test-tucksecret.yaml`

```powershell
kubectl apply -f deploy/examples/test-tucksecret.yaml
Start-Sleep -Seconds 35
kubectl get tucksecret db-password
kubectl get secret myapp-db-credentials -o jsonpath='{.data.password}' | `
  ForEach-Object { [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($_)) }
```

## 7. Unit-тесты и автопрогон

```powershell
go test ./...
.\scripts\run-all-tests.ps1   # все тесты + отчёт в docs/TEST_RESULTS.md
```

## 8. OpenShift Console (настоящий Red Hat UI)

На minikube поднимается **origin-console** — тот же веб-интерфейс, что в OpenShift
(красный брендинг, Developer perspective, Topology). Манифесты: `deploy/console/`.

```powershell
.\scripts\start-openshift-console.ps1
```

Откроется **http://localhost:9000** (окно PowerShell с port-forward держать открытым).

Ручной запуск:

```powershell
kubectl apply -f deploy/console/rbac.yaml
$token = kubectl create token console -n openshift-console --duration=8760h
kubectl create secret generic console-token -n openshift-console --from-literal=token=$token --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/console/deployment.yaml
kubectl -n openshift-console port-forward svc/console 9000:9000
```

**Что смотреть:** namespace `tuck`, CRD `TuckSecret`, Secrets.

> Полный OpenShift (Operators, встроенный OAuth, `crc console`) — [OpenShift Local](https://console.redhat.com/openshift/create/local), ~16 GB RAM, отдельный кластер (не minikube).
>
> Kubernetes Dashboard / Headlamp — запасные варианты: `minikube dashboard`, `minikube service headlamp -n headlamp`.

## Остановка

```powershell
minikube stop    # приостановить кластер
minikube delete  # удалить полностью
```

Полный список сценариев: [TESTING.md](TESTING.md).
