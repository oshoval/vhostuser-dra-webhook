package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	metadataPath = "/var/run/kubernetes.io/dra-device-attributes"
	volumeName   = "groutdra-device-metadata"
)

func main() {
	certFile := flag.String("tls-cert-file", "/tls/tls.crt", "TLS certificate")
	keyFile := flag.String("tls-private-key-file", "/tls/tls.key", "TLS private key")
	addr := flag.String("addr", ":8443", "listen address")
	flag.Parse()

	http.HandleFunc("/mutate", mutate)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	server := &http.Server{Addr: *addr, TLSConfig: tlsConfig}
	log.Printf("listening on %s", *addr)
	log.Fatal(server.ListenAndServeTLS(*certFile, *keyFile))
}

func mutate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil || review.Request == nil {
		http.Error(w, "invalid AdmissionReview", http.StatusBadRequest)
		return
	}

	response := &admissionv1.AdmissionResponse{UID: review.Request.UID, Allowed: true}
	if review.Request.Kind.Kind == "Pod" && review.Request.Operation == admissionv1.Create {
		var pod corev1.Pod
		if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
			response.Allowed = false
			response.Result = &metav1.Status{Message: fmt.Sprintf("decode Pod: %v", err)}
		} else if patch := podPatch(&pod); len(patch) > 0 {
			patchBytes, err := json.Marshal(patch)
			if err != nil {
				response.Allowed = false
				response.Result = &metav1.Status{Message: fmt.Sprintf("marshal patch: %v", err)}
			} else {
				patchType := admissionv1.PatchTypeJSONPatch
				response.PatchType = &patchType
				response.Patch = patchBytes
				log.Printf("mutating launcher Pod %s/%s", pod.Namespace, pod.Name)
			}
		}
	}

	result := admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"}, Response: response}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("write response: %v", err)
	}
}

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func podPatch(pod *corev1.Pod) []patchOp {
	hookIndex := -1
	for i, container := range pod.Spec.Containers {
		if container.Name == "hook-sidecar-0" {
			hookIndex = i
			break
		}
	}
	if hookIndex == -1 || hasMount(pod.Spec.Containers[hookIndex]) {
		return nil
	}

	patch := make([]patchOp, 0, 2)
	if !hasVolume(pod.Spec.Volumes) {
		patch = append(patch, patchOp{Op: "add", Path: "/spec/volumes/-", Value: corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
				Path: metadataPath,
			}},
		}})
	}
	patch = append(patch, patchOp{Op: "add", Path: "/spec/containers/" + strconv.Itoa(hookIndex) + "/volumeMounts/-", Value: corev1.VolumeMount{
		Name: volumeName, MountPath: metadataPath, ReadOnly: true,
	}})
	return patch
}

func hasVolume(volumes []corev1.Volume) bool {
	for _, volume := range volumes {
		if volume.Name == volumeName {
			return true
		}
	}
	return false
}

func hasMount(container corev1.Container) bool {
	for _, mount := range container.VolumeMounts {
		if mount.MountPath == metadataPath {
			return true
		}
	}
	return false
}
