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
	claimName   = "grout-vhu"
	requestName = "vhu"
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
	if hookIndex == -1 || !hasPodClaim(pod.Spec.ResourceClaims) || hasClaim(pod.Spec.Containers[hookIndex]) {
		return nil
	}

	path := "/spec/containers/" + strconv.Itoa(hookIndex) + "/resources/claims"
	claim := corev1.ResourceClaim{Name: claimName, Request: requestName}
	if len(pod.Spec.Containers[hookIndex].Resources.Claims) == 0 {
		return []patchOp{{Op: "add", Path: path, Value: []corev1.ResourceClaim{claim}}}
	}
	return []patchOp{{Op: "add", Path: path + "/-", Value: claim}}
}

func hasPodClaim(claims []corev1.PodResourceClaim) bool {
	for _, claim := range claims {
		if claim.Name == claimName {
			return true
		}
	}
	return false
}

func hasClaim(container corev1.Container) bool {
	for _, claim := range container.Resources.Claims {
		if claim.Name == claimName && claim.Request == requestName {
			return true
		}
	}
	return false
}
