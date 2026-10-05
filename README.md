# Vhost-user DRA hook-sidecar mount webhook

This is an OCP POC workaround for KubeVirt vhost-user bindings backed by DRA.
The DRA driver writes request metadata on the node at
`/var/run/kubernetes.io/dra-device-attributes`. KubeVirt passes the generated
CDI specification to the launcher, but the binding plugin runs in
`hook-sidecar-0`; that sidecar does not receive this metadata mount.

The webhook mutates only Pods containing a container named `hook-sidecar-0`.
It adds a read-only hostPath mount of that DRA metadata directory to the
container. Other Pods receive no patch. The failure policy is `Ignore`, so it
does not block ordinary Pod creation if the webhook is unavailable.

Build and deploy:

```bash
docker build -t quay.io/oshoval/vhostuser-dra-webhook:dev .
docker push quay.io/oshoval/vhostuser-dra-webhook:dev
./deploy.sh
```

After the webhook is ready, recreate the VMI. Confirm the mutation before
debugging the binding further:

```bash
oc -n default get pod -l kubevirt.io/domain=grout-dra-claim-vmi -o jsonpath='{range .items[0].spec.containers[?(@.name=="hook-sidecar-0")].volumeMounts[*]}{.name}{" "}{.mountPath}{"\n"}{end}'
```

This is intentionally a narrow POC workaround. The product-grade resolution
belongs in KubeVirt: propagate DRA metadata mounts to the binding hook sidecar
when it consumes DRA-backed vhost-user metadata.
