package workspace

import (
	"luckytv/backend/internal/providerpreset"
	"testing"
)

func TestBrandPresetPreservesLegacyOriginAndReferences(t *testing.T) {
	for _, origin := range []string{"", "https://legacy-enterprise.example.com", "https://luckytv.app"} {
		channel, err := presetChannelMap(providerpreset.LuckyTV(), map[string]any{"baseUrl": origin, "apiKey": "stored", "models": []any{"custom-model"}})
		if err != nil {
			t.Fatal(err)
		}
		if channel["baseUrl"] != "https://luckytv.app" || channel["name"] != "LuckyTV" || channel["id"] != "luckytv" || channel["apiKey"] != "stored" {
			t.Fatalf("migration changed channel: %#v", channel)
		}
		if models := channel["models"].([]string); len(models) != 1 || models[0] != "custom-model" {
			t.Fatalf("models = %#v", models)
		}
	}
}
