# Vhost-user DRA hook-sidecar mount webhook

This is an OCP 4.23 POC workaround for KubeVirt vhost-user bindings backed by
DRA. The binding plugin runs in `hook-sidecar-0`, but this CNV release does
not add its DRA claim to that container. Consequently kubelet/CDI only mounts
the metadata into `compute`.

The webhook mutates only Pods that contain `hook-sidecar-0` and the POC's
`grout-vhu` Pod resource claim. It adds the claim request `vhu` to the
sidecar's `resources.claims`. Kubelet then applies the driver's CDI metadata
mount to that sidecar, including the correct SELinux labeling. Other Pods
receive no patch. The failure policy is `Ignore`, so it does not block
ordinary Pod creation if the webhook is unavailable.

Build and deploy:

```bash
docker build -t quay.io/oshoval/vhostuser-dra-webhook:dev .
docker push quay.io/oshoval/vhostuser-dra-webhook:dev
./deploy.sh
```

After the webhook is ready, recreate the VMI. Confirm the mutation before
debugging the binding further:

```bash
oc -n default get pod -l kubevirt.io/domain=grout-dra-claim-vmi -o jsonpath='{range .items[0].spec.containers[?(@.name=="hook-sidecar-0")].resources.claims[*]}{.name}{" "}{.request}{"\n"}{end}'
```

This is intentionally a narrow POC backport. Current upstream KubeVirt already
propagates `HookSidecar.ResourceClaims` into the generated hook-sidecar
container; the durable resolution is to make that KubeVirt implementation
available in the CNV release.
