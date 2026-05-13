#!/usr/bin/env bash
# Build the SealedSecret for go-liga and write it to
# deploy/overlays/production/sealed-secret.yaml. Run from a host that
# can reach the k3s cluster (kubeseal needs the cluster's sealing key).
#
# Required env vars:
#   GO_LIGA_OIDC_CLIENT_ID      — from `tofu output -raw goliga_oidc_client_id`
#   GO_LIGA_OIDC_CLIENT_SECRET  — copied from the Zitadel UI when the app
#                                 was created (one-time reveal)
#
# Optional:
#   GO_LIGA_SIGNING_KEY  — 64 hex chars; generated if unset
#   GO_LIGA_MCP_TOKEN    — random bearer; generated if unset
#   GO_LIGA_MCP_USER     — OIDC subject the MCP token acts as (your user)
set -euo pipefail

: "${GO_LIGA_OIDC_CLIENT_ID:?set this from `tofu output -raw goliga_oidc_client_id`}"
: "${GO_LIGA_OIDC_CLIENT_SECRET:?copy from Zitadel UI on app creation}"
: "${GO_LIGA_MCP_USER:?set to your Zitadel user subject (sub claim)}"

GO_LIGA_SIGNING_KEY="${GO_LIGA_SIGNING_KEY:-$(head -c 32 /dev/urandom | xxd -p -c 64)}"
GO_LIGA_MCP_TOKEN="${GO_LIGA_MCP_TOKEN:-$(head -c 24 /dev/urandom | xxd -p)}"

out=deploy/overlays/production/sealed-secret.yaml

kubectl create secret generic go-liga-secret -n go-liga \
  --from-literal=GO_LIGA_SIGNING_KEY="${GO_LIGA_SIGNING_KEY}" \
  --from-literal=GO_LIGA_OIDC_CLIENT_ID="${GO_LIGA_OIDC_CLIENT_ID}" \
  --from-literal=GO_LIGA_OIDC_CLIENT_SECRET="${GO_LIGA_OIDC_CLIENT_SECRET}" \
  --from-literal=GO_LIGA_MCP_TOKEN="${GO_LIGA_MCP_TOKEN}" \
  --from-literal=GO_LIGA_MCP_USER="${GO_LIGA_MCP_USER}" \
  --dry-run=client -o yaml \
  | kubeseal --controller-namespace=kube-system \
             --controller-name=sealed-secrets-controller \
             --format=yaml \
  > "${out}"

echo "wrote ${out}"
echo "commit and push — Flux will apply it."
echo
echo "Generated MCP token (give this to Claude.ai):"
echo "  ${GO_LIGA_MCP_TOKEN}"
