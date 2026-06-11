# Deploy manifests

Apply order for minikube / local k8s:

```powershell
kubectl apply -f deploy/crd/
kubectl apply -f deploy/server/
kubectl apply -f deploy/operator/deployment.yaml
kubectl apply -f deploy/operator/local.yaml      # minikube: local images
kubectl apply -f deploy/console/                 # optional: OpenShift Console UI
```

| Path | Contents |
|------|----------|
| `crd/` | TuckSecret CRD |
| `server/` | Tuck server Deployment, Service, PVC, RBAC |
| `operator/` | Operator Deployment + minikube overlay |
| `console/` | OpenShift Console (origin-console) for minikube |
| `examples/` | Sample TuckSecret resources |

Build images: `build/Dockerfile.server`, `build/Dockerfile.operator`
