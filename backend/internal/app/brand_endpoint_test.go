package app

import (
	"context"
	"testing"
)

func TestBothDesktopBrandsKeepSeedanceContracts(t *testing.T) {
	origin := "https://luckytv.app"
	config := providerConfig{BaseURL: origin, Model: "seedance-2.0", InterfaceType: "newapi"}
	if !isLuckyTVVideoConfig(context.Background(), config) || !isLuckyTVSeedancePreuploadConfig(context.Background(), config) {
		t.Fatalf("missing video or preupload contract for %s", origin)
	}
	if got := providerDownloadURL(origin, "https://luckytv.app/v1/videos/task/content"); got != origin+"/v1/videos/task/content" {
		t.Fatalf("download origin = %s", got)
	}
	legacy := providerConfig{BaseURL: "https://legacy-enterprise.example.com", Model: "seedance-2.0", InterfaceType: "newapi"}
	if isLuckyTVVideoConfig(context.Background(), legacy) || isLuckyTVSeedancePreuploadConfig(context.Background(), legacy) {
		t.Fatal("legacy legacy-enterprise.example.com origin was treated as official endpoint")
	}
}
