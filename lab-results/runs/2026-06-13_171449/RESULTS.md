# Lab test run 2026-06-13_171449

**Date:** 2026-06-13 17:16:12
**Host:** Windows
**Go:** go version go1.25.11 windows/amd64

## Summary

| Metric | Value |
|--------|-------|
| Passed | 27 |
| Failed | 6 |
| Total | 33 |

## Results

| ID | Test | Env | Result | Details |
|----|------|-----|--------|---------|
| UNIT | go test ./... | go | FAIL | exit=1 |
| UNIT-LIST | API list=true compat tests | go | FAIL |  |
| BUILD | go build tuck.exe | build | PASS |  |
| L-SETUP | Local server + root token | local:8202 | PASS | tuck_nsuZsmNW2Bg4QpFLD6k8jmwlNBCZccGjUceckpVHGWA |
| L-READY | Health endpoint | local | PASS |  |
| 1 | Health sealed=false | local | PASS | {"build_date":"unknown","commit":"unknown","ha_enabled":false,"sealed":false,"uptime_seconds":2.0... |
| 2 | Seal status dev | local | PASS | {"sealed":false,"type":"dev"} |
| 3a | KV put | local | PASS | status 204 |
| 3b | KV get | local | PASS | {"created_at":"2026-06-13T14:15:15Z","path":"db/password","value":"supersecret123"} |
| 3c | KV 404 | local | PASS | status 404 |
| 3d | KV delete | local | PASS | status 204 |
| 4a | Auth no token | local | PASS | status 401 |
| 4b | Auth bad token | local | PASS | status 401 |
| 5 | Policy ACL | local | PASS | {"name":"prod-readonly","rules":[{"path":"secret/prod/*","capabilities":["read"]}],"inheritable":... |
| 6 | Token create | local | PASS | tuck_jdoS744FMavrDjqXRJtjBO_Y0SKBV-thg2yf9qMpq5s |
| 7a | Manual seal | local | PASS | status 200 |
| 7b | Sealed 503 | local | PASS | status 503 |
| 10 | Persist restart | local | PASS | {"created_at":"2026-06-13T14:15:17Z","path":"persist-test","value":"persist-ok"} |
| UI-LIST | GET ?list=true Explorer | local | PASS | {"keys":["app/","persist-test"]} |
| UI-KV2 | KV v2 write | local | PASS | status 200 |
| UI-KV2-LIST | KV v2 metadata list | local | PASS | {"keys":["ui-test"]} |
| K8S-PF | Port-forward :8201 | minikube | PASS |  |
| K8S-TOKEN | Root token file | minikube | PASS | tuck_rvBQkofbZzFzYuG... |
| K8S-1 | Health | minikube | PASS | {"sealed":false} |
| 8 | K8s SA auth login+read | minikube | FAIL | {"token":"tuck_3v-a3u89O7fPBwQUUZvCLZwklJTHlo6AbDYBSmxR0W0"} |
| 9a | PUT secret for operator | minikube | PASS | status 204 |
| 9b | TuckSecret operator sync | minikube | FAIL | got=rotated-key-xui want=lab-run-2026-06-13_171449 |
| 9c | TuckSecret CRD exists | minikube | PASS |  |
| UI-K8S-LIST | K8s UI list=true | minikube | FAIL | {"error":"not found"} |
| K8S-PODS | tuck-server pod Running | minikube | PASS |  |
| K8S-OP | tuck-operator Running | minikube | PASS |  |
| CLUSTER | minikube node Ready | minikube | FAIL |  |
| CRD | TuckSecret CRD installed | minikube | PASS |  |

## Artifacts

- go-test.log - full unit test output
- results.json - machine-readable
