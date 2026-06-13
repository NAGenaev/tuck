# Lab test run 2026-06-13_192554

**Date:** 2026-06-13 19:26:51
**Host:** Windows
**Go:** go version go1.25.11 windows/amd64

## Summary

| Metric | Value |
|--------|-------|
| Passed | 32 |
| Failed | 0 |
| Total | 32 |

## Results

| ID | Test | Env | Result | Details |
|----|------|-----|--------|---------|
| UNIT | go test ./... (skipped) | go | PASS | use -SkipUnit to bypass |
| BUILD | go build tuck.exe | build | PASS |  |
| L-SETUP | Local server + root token | local:8202 | PASS | tuck_Yxtd0-E0HAa81tMi1QQi6ubN14vaLy3ooQmaU0vxgdc |
| L-READY | Health endpoint | local | PASS |  |
| 1 | Health sealed=false | local | PASS | {"build_date":"unknown","commit":"unknown","ha_enabled":false,"sealed":false,"uptime_seconds":2.0... |
| 2 | Seal status dev | local | PASS | {"sealed":false,"type":"dev"} |
| 3a | KV put | local | PASS | status 204 |
| 3b | KV get | local | PASS | {"created_at":"2026-06-13T16:25:58Z","path":"db/password","value":"supersecret123"} |
| 3c | KV 404 | local | PASS | status 404 |
| 3d | KV delete | local | PASS | status 204 |
| 4a | Auth no token | local | PASS | status 401 |
| 4b | Auth bad token | local | PASS | status 401 |
| 5 | Policy ACL | local | PASS | {"name":"prod-readonly","rules":[{"path":"secret/prod/*","capabilities":["read"]}],"inheritable":... |
| 6 | Token create | local | PASS | tuck_AOPoX2y-e4TDma3NVAXjtqYYe84km0DBdPWfiDO6kM0 |
| 7a | Manual seal | local | PASS | status 200 |
| 7b | Sealed 503 | local | PASS | status 503 |
| 10 | Persist restart | local | PASS | {"created_at":"2026-06-13T16:25:59Z","path":"persist-test","value":"persist-ok"} |
| UI-LIST | GET ?list=true Explorer | local | PASS | {"keys":["app/","persist-test"]} |
| UI-KV2 | KV v2 write | local | PASS | status 200 |
| UI-KV2-LIST | KV v2 metadata list | local | PASS | {"keys":["ui-test"]} |
| K8S-PF | Port-forward :8201 | minikube | PASS |  |
| K8S-TOKEN | Root token file | minikube | PASS | tuck_wbyuM95TpPw3I10... |
| K8S-1 | Health | minikube | PASS | {"build_date":"unknown","commit":"unknown","ha_enabled":false,"sealed":false,"uptime_seconds":255... |
| 8 | K8s SA auth login+read | minikube | PASS | ok |
| 9a | PUT secret for operator | minikube | PASS | status 204 |
| 9b | TuckSecret operator sync | minikube | PASS | got=lab-run-2026-06-13_192554 want=lab-run-2026-06-13_192554 |
| 9c | TuckSecret CRD exists | minikube | PASS |  |
| UI-K8S-LIST | K8s UI list=true | minikube | PASS | {"keys":["app/","demo-app/"]} |
| K8S-PODS | tuck-server pod Running | minikube | PASS |  |
| K8S-OP | tuck-operator Running | minikube | PASS |  |
| CLUSTER | minikube node Ready | minikube | PASS | True |
| CRD | TuckSecret CRD installed | minikube | PASS |  |

## Artifacts

- go-test.log - full unit test output
- results.json - machine-readable
