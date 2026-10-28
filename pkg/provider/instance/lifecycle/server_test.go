package lifecycle

import (
	"testing"

	"github.com/bizflycloud/gobizfly"
)

func karpenterServer(id, name, clusterID, status string) *gobizfly.Server {
	return &gobizfly.Server{ID: id, Name: name, Status: status, Metadata: map[string]string{
		"karpenter-managed": "true",
		"bke_cluster_id":    clusterID,
	}}
}

// A server another Create just made, in this or another cluster, must never be
// taken for the server of this NodeClaim.
func TestFindClusterServerMatchesNameAndCluster(t *testing.T) {
	servers := []*gobizfly.Server{
		karpenterServer("other-claim", "default-aaaaa", "cluster-a", "ACTIVE"),
		karpenterServer("other-cluster", "default-bbbbb", "cluster-b", "ACTIVE"),
		karpenterServer("failed", "default-bbbbb", "cluster-a", "ERROR"),
		{ID: "pool-node", Name: "default-bbbbb", Status: "ACTIVE"},
		karpenterServer("mine", "default-bbbbb", "cluster-a", "BUILD"),
	}
	if got := findClusterServer(servers, "default-bbbbb", "cluster-a"); got == nil || got.ID != "mine" {
		t.Fatalf("findClusterServer() = %v, want server mine", got)
	}
	if got := findClusterServer(servers[:4], "default-bbbbb", "cluster-a"); got != nil {
		t.Fatalf("findClusterServer() = %s, want none before the server shows up", got.ID)
	}
}
