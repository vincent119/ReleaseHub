package application

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestRuntimeTopologyNodeLimitPreservesValidSubgraph(t *testing.T) {
	for _, count := range []int{maxTopologyNodes, maxTopologyNodes + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			resources := make([]argodomain.RuntimeResource, count)
			for i := range resources {
				resources[i].Ref = runtimeRef("", "v1", "Pod", "test", fmt.Sprintf("pod-%04d", i))
				if i > 0 {
					resources[i].ParentRefs = []argodomain.RuntimeResourceRef{resources[i-1].Ref}
				}
			}
			resources[0].ParentRefs = []argodomain.RuntimeResourceRef{resources[count-1].Ref}
			value := topologyForLimits(t, resources)
			limited := count > maxTopologyNodes
			if len(value.Nodes) != maxTopologyNodes || value.Partial != limited || slices.Contains(value.Warnings, "node_limit") != limited {
				t.Fatalf("nodes=%d, partial=%t, warnings=%v", len(value.Nodes), value.Partial, value.Warnings)
			}
			wantEdges := maxTopologyNodes
			if limited {
				wantEdges--
			}
			if len(value.Edges) != wantEdges {
				t.Fatalf("edges=%d, want %d", len(value.Edges), wantEdges)
			}
			assertRuntimeEdgeEndpoints(t, value)
			if len(resources) != count || len(resources[0].ParentRefs) != 1 {
				t.Fatal("source tree was modified")
			}
		})
	}
}

func TestRuntimeTopologyEdgeLimitPreservesValidSubgraph(t *testing.T) {
	for _, count := range []int{maxTopologyEdges, maxTopologyEdges + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			resources := make([]argodomain.RuntimeResource, 50)
			for i := range resources {
				resources[i].Ref = runtimeRef("", "v1", "Pod", "test", fmt.Sprintf("pod-%02d", i))
			}
			remaining := count
			for i := range resources {
				for j := range resources {
					if i == j || remaining == 0 {
						continue
					}
					resources[i].ParentRefs = append(resources[i].ParentRefs, resources[j].Ref)
					remaining--
				}
			}
			value := topologyForLimits(t, resources)
			limited := count > maxTopologyEdges
			if len(value.Nodes) != 50 || len(value.Edges) != maxTopologyEdges || value.Partial != limited || slices.Contains(value.Warnings, "edge_limit") != limited {
				t.Fatalf("nodes=%d, edges=%d, partial=%t, warnings=%v", len(value.Nodes), len(value.Edges), value.Partial, value.Warnings)
			}
			assertRuntimeEdgeEndpoints(t, value)
		})
	}
}

func topologyForLimits(t *testing.T, resources []argodomain.RuntimeResource) RuntimeTopology {
	t.Helper()
	application := runtimeApplication()
	reader := &runtimeReaderStub{tree: argodomain.RuntimeTree{Resources: resources}}
	service := newRuntimeServiceForTest(t, &runtimeCatalogStub{application: application}, reader, &runtimeAuditorStub{})
	value, err := service.Topology(t.Context(), RuntimePrincipal{UserID: uuid.New()}, application.ID, "resources")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func assertRuntimeEdgeEndpoints(t *testing.T, value RuntimeTopology) {
	t.Helper()
	visible := make(map[string]bool, len(value.Nodes))
	for _, node := range value.Nodes {
		visible[node.Resource.Ref.Key()] = true
	}
	seen := make(map[string]bool, len(value.Edges))
	for _, edge := range value.Edges {
		if !visible[edge.Source] || !visible[edge.Target] || seen[edge.ID] {
			t.Fatalf("invalid edge: %#v", edge)
		}
		seen[edge.ID] = true
	}
}
