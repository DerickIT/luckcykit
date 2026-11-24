package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestWeChatProviderRequiresUnionIDAndUsesConfiguredOAuthEndpoints(t *testing.T) {
	var requested url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","openid":"openid","unionid":"union"}`))
	}))
	defer server.Close()

	provider := WeChatProvider{AppID: "app-id", AppSecret: "app-secret", RedirectURI: "https://cloud.example.com/api/v1/auth/wechat/callback", TokenURL: server.URL}
	identity, err := provider.Resolve(context.Background(), "oauth-code")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Provider != "wechat_unionid" || identity.Subject != "union" {
		t.Fatalf("unexpected provider identity: %+v", identity)
	}
	if requested.Get("appid") != "app-id" || requested.Get("secret") != "app-secret" || requested.Get("code") != "oauth-code" {
		t.Fatalf("unexpected token exchange query: %v", requested)
	}

	state := "random-state"
	authorizationURL, err := provider.AuthorizationURL(state, provider.RedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("appid") != "app-id" || parsed.Query().Get("state") != state || parsed.Query().Get("scope") != "snsapi_login" {
		t.Fatalf("unexpected authorization URL: %s", authorizationURL)
	}
}

func TestWeChatProviderRejectsMissingUnionID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"access","openid":"openid"}`))
	}))
	defer server.Close()
	provider := WeChatProvider{AppID: "app-id", AppSecret: "app-secret", RedirectURI: "https://cloud.example.com/callback", TokenURL: server.URL}
	if _, err := provider.Resolve(context.Background(), "oauth-code"); err != ErrWeChatUnionIDRequired {
		t.Fatalf("missing UnionID was accepted: %v", err)
	}
}
