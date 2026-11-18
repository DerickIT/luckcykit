package modelcatalog

import "testing"

// Canvas nodes saved before v2.2.0 name the official channel by its former ID.
// After reconnecting removes that entry, the assistant must still propose them
// on the official channel, not report the model unavailable or pick another.
func TestAssistantResolvesNodesSavedUnderTheFormerOfficialID(t *testing.T) {
	snapshot := AssistantConfigSnapshot{Channels: []AssistantChannel{
		{ID: "channel-2", Enabled: true, Models: []string{"shared-image"}, ModelProfiles: []AssistantModelProfile{{Model: "shared-image", Capability: "image", Protocol: "openai-image"}}},
		{ID: "luckytv", Enabled: true, Pinned: true, LegacyIDs: []string{"luckyapi"}, Models: []string{"shared-image"}, ModelProfiles: []AssistantModelProfile{{Model: "shared-image", Capability: "image", Protocol: "openai-image"}}},
	}}
	choice := ResolveAssistantGenerationModel(snapshot, "image", "luckyapi::shared-image")
	if choice.ModelKey != "luckytv::shared-image" || !choice.FromNode {
		t.Fatalf("choice = %#v", choice)
	}
	// The current ID still wins over a legacy one.
	snapshot.Channels = append(snapshot.Channels, AssistantChannel{ID: "luckyapi", Enabled: true, Models: []string{"shared-image"}, ModelProfiles: []AssistantModelProfile{{Model: "shared-image", Capability: "image", Protocol: "openai-image"}}})
	if choice := ResolveAssistantGenerationModel(snapshot, "image", "luckyapi::shared-image"); choice.ModelKey != "luckyapi::shared-image" {
		t.Fatalf("current ID did not win: %#v", choice)
	}
}

func TestAssistantChatModelSavedUnderTheFormerOfficialIDUsesTheOfficialChannel(t *testing.T) {
	channels := []AssistantChannel{
		{ID: "channel-2", Enabled: true, Models: []string{"chat-x"}},
		{ID: "luckytv", Enabled: true, LegacyIDs: []string{"luckyapi"}, Models: []string{"chat-x"}},
	}
	if channel, ok := FindAssistantChannel(channels, "luckyapi", "chat-x"); !ok || channel.ID != "luckytv" {
		t.Fatalf("channel = %#v ok=%v", channel, ok)
	}
}
