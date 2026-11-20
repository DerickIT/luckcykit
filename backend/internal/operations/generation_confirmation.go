package operations

import (
	"encoding/json"
	"strings"
)

// ResolveConfirmationTargets validates a user confirmation against the turn's
// focused node and the proposal that was actually shown. It returns the targets
// the caller may submit: the explicit selection, or the stored plan when the
// caller confirms without repeating targets.
func ResolveConfirmationTargets(focusedNodeID string, nodeIDs []string, requested []GenerationTarget, proposalID, storedAuthorizationJSON string) ([]GenerationTarget, error) {
	if focusedNodeID != "" {
		if len(nodeIDs) != 1 || strings.TrimSpace(nodeIDs[0]) != focusedNodeID {
			return nil, PermissionDenied("focused_node_scope", "本轮只能确认当前选中节点的生成方案")
		}
		for _, target := range requested {
			if strings.TrimSpace(target.NodeID) != focusedNodeID {
				return nil, PermissionDenied("focused_node_scope", "本轮只能确认当前选中节点的生成方案")
			}
		}
	}
	if len(requested) > 0 || storedAuthorizationJSON == "" {
		return requested, nil
	}
	var stored GenerationAuthorization
	if json.Unmarshal([]byte(storedAuthorizationJSON), &stored) != nil {
		return requested, nil
	}
	if stored.ProposalID != "" && stored.ProposalID != proposalID {
		return nil, Conflict("production_changed", "这一轮已确认了不同的制作方案", nil)
	}
	return stored.Targets, nil
}