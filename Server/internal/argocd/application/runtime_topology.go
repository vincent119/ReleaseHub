package application

import (
	"slices"
	"strings"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

type topologyEdgeProjection struct {
	edges      []RuntimeTopologyEdge
	evidence   bool
	unresolved bool
}

func boundedTopologyResources(values []argodomain.RuntimeResource) ([]argodomain.RuntimeResource, bool, []string) {
	resources := slices.Clone(values)
	if len(resources) <= maxTopologyNodes {
		return resources, false, nil
	}
	return resources[:maxTopologyNodes], true, []string{"node_limit"}
}

func topologyNodes(resources []argodomain.RuntimeResource) ([]RuntimeTopologyNode, map[string]struct{}) {
	nodes := make([]RuntimeTopologyNode, 0, len(resources))
	visible := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		nodes = append(nodes, RuntimeTopologyNode{Resource: resource})
		visible[resource.Ref.Key()] = struct{}{}
	}
	return nodes, visible
}

func boundedTopologyEdges(values []RuntimeTopologyEdge) ([]RuntimeTopologyEdge, bool, []string) {
	if len(values) <= maxTopologyEdges {
		return values, false, nil
	}
	return values[:maxTopologyEdges], true, []string{"edge_limit"}
}

func applyNetworkProjectionWarnings(view string, projection topologyEdgeProjection, partial bool, warnings []string) (bool, []string) {
	if view != "network" {
		return partial, warnings
	}
	if !projection.evidence {
		warnings = append(warnings, "network_evidence_unavailable")
	}
	if projection.unresolved {
		return true, append(warnings, "network_evidence_unresolved")
	}
	return partial, warnings
}

func topologyEdges(view string, resources []argodomain.RuntimeResource, visible map[string]struct{}) topologyEdgeProjection {
	if view == "network" {
		return networkTopologyEdges(resources)
	}
	return resourceTopologyEdges(resources, visible)
}

func resourceTopologyEdges(resources []argodomain.RuntimeResource, visible map[string]struct{}) topologyEdgeProjection {
	projection := topologyEdgeProjection{}
	for _, resource := range resources {
		for _, parent := range resource.ParentRefs {
			if _, ok := visible[parent.Key()]; ok {
				projection.add("resource", parent.Key(), resource.Ref.Key())
			}
		}
	}
	projection.sort()
	return projection
}

func networkTopologyEdges(resources []argodomain.RuntimeResource) topologyEdgeProjection {
	projection := topologyEdgeProjection{}
	for _, source := range resources {
		projection.evidence = projection.evidence || hasNetworkEvidence(source.Networking)
		projection.addReferenceEdges(source, resources)
		projection.addSelectorEdges(source, resources)
	}
	projection.sort()
	return projection
}

func (p *topologyEdgeProjection) addReferenceEdges(source argodomain.RuntimeResource, resources []argodomain.RuntimeResource) {
	for _, reference := range source.Networking.TargetRefs {
		if strings.TrimSpace(reference.Name) == "" {
			p.unresolved = p.unresolved || len(source.Networking.TargetLabels) == 0
			continue
		}
		matches := matchingReferences(reference, resources)
		if len(matches) != 1 {
			p.unresolved = true
			continue
		}
		p.add("network", source.Ref.Key(), matches[0].Ref.Key())
	}
}

func (p *topologyEdgeProjection) addSelectorEdges(source argodomain.RuntimeResource, resources []argodomain.RuntimeResource) {
	if len(source.Networking.TargetLabels) == 0 {
		return
	}
	matches := matchingSelectors(source, resources)
	p.unresolved = p.unresolved || len(matches) == 0
	for _, target := range matches {
		p.add("network", source.Ref.Key(), target.Ref.Key())
	}
}

func matchingReferences(reference argodomain.RuntimeResourceRef, resources []argodomain.RuntimeResource) []argodomain.RuntimeResource {
	return slices.DeleteFunc(slices.Clone(resources), func(resource argodomain.RuntimeResource) bool {
		return !referenceMatches(reference, resource.Ref)
	})
}

func referenceMatches(reference, candidate argodomain.RuntimeResourceRef) bool {
	return optionalMatch(reference.Group, candidate.Group) && optionalMatch(reference.Version, candidate.Version) &&
		optionalMatch(reference.Kind, candidate.Kind) && optionalMatch(reference.Namespace, candidate.Namespace) &&
		optionalMatch(reference.Name, candidate.Name) && optionalMatch(reference.UID, candidate.UID)
}

func optionalMatch(expected, actual string) bool {
	return expected == "" || expected == actual
}

func matchingSelectors(source argodomain.RuntimeResource, resources []argodomain.RuntimeResource) []argodomain.RuntimeResource {
	return slices.DeleteFunc(slices.Clone(resources), func(candidate argodomain.RuntimeResource) bool {
		return candidate.Ref.Key() == source.Ref.Key() ||
			!labelsMatch(source.Networking.TargetLabels, candidate.Networking.Labels) ||
			!selectorReferenceMatches(source.Networking.TargetRefs, candidate.Ref)
	})
}

func labelsMatch(selector, labels map[string]string) bool {
	if len(selector) == 0 || len(labels) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func selectorReferenceMatches(references []argodomain.RuntimeResourceRef, candidate argodomain.RuntimeResourceRef) bool {
	restrictions := slices.DeleteFunc(slices.Clone(references), func(reference argodomain.RuntimeResourceRef) bool {
		return reference.Name != ""
	})
	if len(restrictions) == 0 {
		return true
	}
	return slices.ContainsFunc(restrictions, func(reference argodomain.RuntimeResourceRef) bool {
		return referenceMatches(reference, candidate)
	})
}

func hasNetworkEvidence(networking argodomain.RuntimeNetworking) bool {
	return len(networking.TargetRefs) > 0 || len(networking.TargetLabels) > 0 ||
		len(networking.Ingress) > 0 || len(networking.ExternalURLs) > 0
}

func (p *topologyEdgeProjection) add(kind, source, target string) {
	id := kind + ":" + source + ":" + target
	if slices.ContainsFunc(p.edges, func(edge RuntimeTopologyEdge) bool { return edge.ID == id }) {
		return
	}
	p.edges = append(p.edges, RuntimeTopologyEdge{ID: id, Source: source, Target: target, Kind: kind})
}

func (p *topologyEdgeProjection) sort() {
	slices.SortFunc(p.edges, func(left, right RuntimeTopologyEdge) int {
		return strings.Compare(left.ID, right.ID)
	})
}
