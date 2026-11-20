package operations

import (
	"errors"
	"testing"
)

func TestResolveConfirmationTargetsEnforcesFocusedNode(t *testing.T) {
	targets := []GenerationTarget{{NodeID: "image"}}
	if _, err := ResolveConfirmationTargets("image", []string{"image", "other"}, targets, "proposal-1", ""); err == nil {
		t.Fatal("multiple selected nodes were accepted under focused-node scope")
	}
	if _, err := ResolveConfirmationTargets("image", []string{"image"}, []GenerationTarget{{NodeID: "other"}}, "proposal-1", ""); err == nil {
		t.Fatal("target outside the focused node was accepted")
	}
	if _, err := ResolveConfirmationTargets("", []string{"image", "other"}, targets, "proposal-1", ""); err != nil {
		t.Fatalf("unfocused confirmation rejected: %v", err)
	}
}

func TestResolveConfirmationTargetsUsesStoredPlan(t *testing.T) {
	stored := `{"sourceTurnId":"turn-1","proposalId":"proposal-1","targets":[{"nodeId":"image","title":"镜头"}]}`
	resolved, err := ResolveConfirmationTargets("", nil, nil, "proposal-1", stored)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].NodeID != "image" {
		t.Fatalf("stored plan was not used: %+v", resolved)
	}

	if _, err := ResolveConfirmationTargets("", nil, nil, "proposal-2", stored); err == nil {
		t.Fatal("confirmation for a different proposal was accepted")
	} else {
		var opErr *Error
		if !errors.As(err, &opErr) || opErr.Reason != "production_changed" {
			t.Fatalf("unexpected conflict error: %v", err)
		}
	}
}