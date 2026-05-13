# Deployment

Go-Liga ist ein einzelnes Go-Binary mit eingebetteter SQLite, deployed auf einem k3s-Cluster via Flux. Es gibt keine separate Service-Komponente, keinen Build-Schritt im Cluster und keinen manuellen `kubectl apply`-Pfad.

## Code-Pfad

1. Du committest und pusht zu `levino/go-ranking` Branch `main` (oder mergst einen PR).
2. GitHub Actions Workflow `.github/workflows/deploy.yml` läuft:
   - Baut ein Multi-Arch-Image (`linux/amd64,linux/arm64`).
   - Pushed nach `ghcr.io/levino/go-ranking:<sha>` und `:latest`.
   - Schreibt den neuen SHA in `deploy/overlays/production/kustomization.yaml` und committet das mit `[skip ci]` zurück auf `main`.
3. Flux-System im Cluster pollt das Repo alle 60 s. Sieht den neuen Commit, ruft `kustomize build` auf, vergleicht mit dem Live-Stand, applied die Änderungen.
4. Kubernetes rolled das Deployment (`strategy: Recreate`, weil SQLite single-writer).

Vom `git push` bis Live: ~3-4 Minuten.

## Warum so?

Die k3s-API ist nicht öffentlich erreichbar — von GitHub Actions aus kann man also nicht direkt `kubectl set image` machen. Das Push-zurück-auf-main-Pattern macht das Repo zur einzigen Quelle der Wahrheit; Flux pullt, niemand pushed zum Cluster.

## Repo-Layout

```
deploy/
├── base/                          (Kustomize-Base — alle Manifeste)
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml               (ranking.go-ag.levinkeller.de + TLS)
│   ├── pvc.yaml                   (1 GiB für die SQLite)
│   ├── rbac.yaml
│   ├── namespace.yaml
│   └── kustomization.yaml
└── overlays/
    └── production/
        ├── kustomization.yaml     (newTag wird vom CI gepflegt)
        └── sealed-secret.yaml     (offline gegen den Cluster-Public-Key gesealed)
```

Flux pickt `./deploy/overlays/production` aus dem Repo, siehe `server-config/apps/go-liga.yaml`.

## Wenn du das Secret rotieren musst

Der SealedSecret im Repo ist verschlüsselt — du kannst ihn nicht direkt editieren. Stattdessen:

1. Neuen Klartext-Secret bauen (z.B. neues Signing-Key, neue OIDC-Creds).
2. Mit `kubeseal --cert server-config/sealed-secrets-public-key.pem` offline neu sealen.
3. `deploy/overlays/production/sealed-secret.yaml` durch das Ergebnis ersetzen.
4. Committen, pushen. Flux applied, Sealed-Secrets-Controller entschlüsselt, die `go-liga-secret`-Secret wird aktualisiert, Pod erkennt das beim nächsten Restart.

Detail-Anleitung in `server-config/services/go-liga/README.md`.

## Wenn ein Deploy fehlschlägt

- **ghcr.io login timeout** (passiert ab und zu): Workflow `gh run rerun` reicht.
- **Pod CrashLoop** nach einem Schema-Update: einen `kubectl -n go-liga logs deploy/go-liga` machen, Fehler diagnostizieren. Bei DB-Migrations-Fehlern liegt die Lösung in `internal/store/store.go: migrate()`.
- **Flux applied nicht**: `kubectl -n flux-system annotate kustomization go-liga reconcile.fluxcd.io/requestedAt=$(date +%s) --overwrite` triggert sofortigen Reconcile.

## Wo läuft was?

| Komponente | Wo |
|---|---|
| App-Pod | k3s, Namespace `go-liga`, ein Replica |
| Persistente Daten | PVC `go-liga-data` (local-path Storage Class) |
| Ingress / TLS | Traefik + cert-manager (Let's Encrypt) |
| Bilder | ghcr.io/levino/go-ranking |
| Authority (OIDC) | id.levinkeller.de (Zitadel im selben Cluster) |
| Sealed-Secrets-Controller | k3s, Namespace `kube-system` |
| Flux | k3s, Namespace `flux-system` |

Server-Spezifika (k3s-Konfig, OIDC-Trust, etc.) in `levino/server-config`.
