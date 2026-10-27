package lifecycle

import (
	"testing"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
)

func TestWorkerImageSkipsVersionsWithoutImage(t *testing.T) {
	m := &Manager{
		Log:                logr.Discard(),
		k8sVersionsFetched: true,
		k8sVersionsResponse: &gobizfly.KubernetesVersionResponse{
			ControllerVersions: []gobizfly.ControllerVersion{
				{K8SVersion: "v1.35.9"},
				{K8SVersion: "v1.35.8"},
				{K8SVersion: "v1.35.5", WorkerImageID: "img-1.35.5"},
				{K8SVersion: "v1.36.1", WorkerImageID: "img-1.36.1"},
			},
		},
	}
	got, err := m.getWorkerImageIDFromAPI("v1.35.9", "ubuntu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "img-1.35.5" {
		t.Fatalf("got image %q, want img-1.35.5", got)
	}
}
