#!/usr/bin/env bash
set -euo pipefail

namespace=grout-poc-system
service=vhostuser-dra-webhook
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

oc create namespace "$namespace" --dry-run=client -o yaml | oc apply -f -

openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "$tmpdir/tls.key" -out "$tmpdir/tls.crt" \
  -subj "/CN=${service}.${namespace}.svc" \
  -addext "subjectAltName=DNS:${service},DNS:${service}.${namespace},DNS:${service}.${namespace}.svc"

oc -n "$namespace" create secret tls "${service}-tls" \
  --cert="$tmpdir/tls.crt" --key="$tmpdir/tls.key" \
  --dry-run=client -o yaml | oc apply -f -

ca_bundle=$(base64 -w0 < "$tmpdir/tls.crt")
sed "s|__CA_BUNDLE__|${ca_bundle}|" deploy.yaml | oc apply -f -
oc -n "$namespace" rollout status deployment/"$service" --timeout=120s
