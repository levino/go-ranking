#!/usr/bin/env bash
# Build the SealedSecret for go-liga and write it to
# deploy/overlays/production/sealed-secret.yaml.
#
# Required env vars:
#   GO_LIGA_OIDC_CLIENT_ID      — `tofu output -raw goliga_oidc_client_id`
#   GO_LIGA_OIDC_CLIENT_SECRET  — `tofu output -raw goliga_oidc_client_secret`
#
# Optional:
#   GO_LIGA_SIGNING_KEY   — 64 hex chars; generated if unset
#   GO_LIGA_SEAL_CERT     — path to sealing public key for offline sealing
#                           (e.g. server-config/sealed-secrets-public-key.pem).
#                           If unset, kubeseal will contact the cluster.
set -euo pipefail

: "${GO_LIGA_OIDC_CLIENT_ID:?set from \`tofu output -raw goliga_oidc_client_id\`}"
: "${GO_LIGA_OIDC_CLIENT_SECRET:?set from \`tofu output -raw goliga_oidc_client_secret\`}"

GO_LIGA_SIGNING_KEY="${GO_LIGA_SIGNING_KEY:-$(head -c 32 /dev/urandom | xxd -p -c 64)}"

out=deploy/overlays/production/sealed-secret.yaml

kubeseal_args=(--format=yaml)
if [[ -n "${GO_LIGA_SEAL_CERT:-}" ]]; then
    kubeseal_args+=(--cert "${GO_LIGA_SEAL_CERT}")
else
    kubeseal_args+=(--controller-namespace=kube-system --controller-name=sealed-secrets-controller)
fi

kubectl create secret generic go-liga-secret -n go-liga \
  --from-literal=GO_LIGA_SIGNING_KEY="${GO_LIGA_SIGNING_KEY}" \
  --from-literal=GO_LIGA_OIDC_CLIENT_ID="${GO_LIGA_OIDC_CLIENT_ID}" \
  --from-literal=GO_LIGA_OIDC_CLIENT_SECRET="${GO_LIGA_OIDC_CLIENT_SECRET}" \
  --dry-run=client -o yaml \
  | kubeseal "${kubeseal_args[@]}" \
  > "${out}"

echo "wrote ${out}"
echo "commit and push — Flux will apply it."
