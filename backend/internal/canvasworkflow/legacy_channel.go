package canvasworkflow

import (
	"encoding/json"
	"strings"

	"luckytv/backend/internal/luckytv"
	"luckytv/backend/internal/providerpreset"
)

// officialLegacyIDs are IDs the official channel was saved under in earlier
// releases (before v2.2.0 it was "luckyapi"), declared by the official preset.
func officialLegacyIDs() []string {
	for _, preset := range providerpreset.BuiltinChannels() {
		if preset.ID == luckytv.ChannelID {
			return preset.LegacyIDs
		}
	}
	return nil
}

// IsOfficialWorkflowChannel reports whether a saved workflow recipe runs on the
// official channel, including recipes saved under a former official ID.
func IsOfficialWorkflowChannel(channelID string) bool {
	if channelID == "@luckytv" {
		return true
	}
	for _, legacy := range officialLegacyIDs() {
		if channelID == "@"+legacy {
			return true
		}
	}
	return false
}

// UnmarshalJSON reads every saved workflow node through one place, so a node
// saved before the official channel was renamed ("luckyapi::model" or
// channelId "luckyapi") runs and displays on the current official channel.
func (n *Node) UnmarshalJSON(data []byte) error {
	type plain Node
	if err := json.Unmarshal(data, (*plain)(n)); err != nil {
		return err
	}
	if n.Config == nil {
		return nil
	}
	for _, legacy := range officialLegacyIDs() {
		if option, ok := n.Config["modelOption"].(string); ok && strings.HasPrefix(option, legacy+"::") {
			n.Config["modelOption"] = luckytv.ChannelID + option[len(legacy):]
		}
		if channel, ok := n.Config["channelId"].(string); ok && channel == legacy {
			n.Config["channelId"] = luckytv.ChannelID
		}
	}
	return nil
}
