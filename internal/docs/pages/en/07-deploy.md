# Deployment

Go-Liga is a single Go binary with embedded SQLite, deployed on a k3s cluster via Flux. There is no separate service component, no build step in the cluster, and no manual `kubectl apply` path.

## Code path

1. You commit and push to `levino/go-ranking` branch `main` (or merge a PR).
2. The GitHub Actions workflow `.github/workflows/deploy.yml` runs:
   - Builds a multi-arch image (`linux/amd64,linux/arm64`).
   - Pushes to `ghcr.io/levino/go-ranking:<sha>` and `:latest`.
   - Writes the new SHA into `deploy/overlays/production/kustomization.yaml` and commits it back to `main` with `[skip ci]`.
3. The Flux system in the cluster polls the repo every 60 s. It sees the new commit, calls `kustomize build`, compares with the live state, and applies the changes.
4. Kubernetes rolls the deployment (`strategy: Recreate`, because SQLite is single-writer).

From `git push` to live: ~3-4 minutes.

## Why this way?

The k3s API is not publicly reachable — so from GitHub Actions you can't run `kubectl set image` directly. The push-back-to-main pattern makes the repo the single source of truth; Flux pulls, no one pushes to the cluster.

## Repo layout

```
deploy/
├── base/                          (Kustomize base — all manifests)
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml               (ranking.go-ag.levinkeller.de + TLS)
│   ├── pvc.yaml                   (1 GiB for the SQLite)
│   ├── rbac.yaml
│   ├── namespace.yaml
│   └── kustomization.yaml
└── overlays/
    └── production/
        ├── kustomization.yaml     (newTag is maintained by CI)
        └── sealed-secret.yaml     (sealed offline against the cluster public key)
```

Flux picks `./deploy/overlays/production` from the repo, see `server-config/apps/go-liga.yaml`.

## When you need to rotate the secret

The SealedSecret in the repo is encrypted — you can't edit it directly. Instead:

1. Build a new plaintext secret (e.g. a new signing key, new OIDC creds).
2. Re-seal it offline with `kubeseal --cert server-config/sealed-secrets-public-key.pem`.
3. Replace `deploy/overlays/production/sealed-secret.yaml` with the result.
4. Commit, push. Flux applies, the Sealed Secrets controller decrypts, the `go-liga-secret` secret is updated, and the pod picks it up on the next restart.

Detailed instructions in `server-config/services/go-liga/README.md`.

## When a deploy fails

- **ghcr.io login timeout** (happens now and then): a workflow `gh run rerun` is enough.
- **Pod CrashLoop** after a schema update: run a `kubectl -n go-liga logs deploy/go-liga`, diagnose the error. For DB migration errors the fix lives in `internal/store/store.go: migrate()`.
- **Flux not applying**: `kubectl -n flux-system annotate kustomization go-liga reconcile.fluxcd.io/requestedAt=$(date +%s) --overwrite` triggers an immediate reconcile.

## Where does what run?

| Component | Where |
|---|---|
| App pod | k3s, namespace `go-liga`, one replica |
| Persistent data | PVC `go-liga-data` (local-path storage class) |
| Ingress / TLS | Traefik + cert-manager (Let's Encrypt) |
| Images | ghcr.io/levino/go-ranking |
| Authority (OIDC) | id.levinkeller.de (Zitadel in the same cluster) |
| Sealed Secrets controller | k3s, namespace `kube-system` |
| Flux | k3s, namespace `flux-system` |

Server specifics (k3s config, OIDC trust, etc.) in `levino/server-config`.
