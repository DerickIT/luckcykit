package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrWeChatUnionIDRequired = errors.New("wechat unionid is required")

type WeChatProvider struct {
	AppID        string
	AppSecret    string
	RedirectURI  string
	HTTPClient   *http.Client
	AuthorizeURL string
	TokenURL     string
}

type weChatTokenResponse struct {
	AccessToken string `json:"access_token"`
	OpenID      string `json:"openid"`
	UnionID     string `json:"unionid"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

func (p WeChatProvider) AuthorizationURL(state, redirectURI string) (string, error) {
	if strings.TrimSpace(p.AppID) == "" || strings.TrimSpace(state) == "" {
		return "", errors.New("wechat app id and OAuth state are required")
	}
	redirect, err := url.Parse(redirectURI)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil || redirect.RawQuery != "" || redirect.Fragment != "" {
		return "", errors.New("wechat redirect URI must be an HTTPS URL without credentials, query, or fragment")
	}
	authorize := p.AuthorizeURL
	if authorize == "" {
		authorize = "https://open.weixin.qq.com/connect/qrconnect"
	}
	values := url.Values{}
	values.Set("appid", p.AppID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", "snsapi_login")
	values.Set("state", state)
	return authorize + "?" + values.Encode() + "#wechat_redirect", nil
}

func (p WeChatProvider) Resolve(ctx context.Context, code string) (ProviderIdentity, error) {
	if strings.TrimSpace(p.RedirectURI) == "" {
		return ProviderIdentity{}, errors.New("wechat OAuth redirect URI is not configured")
	}
	if strings.TrimSpace(p.AppID) == "" || strings.TrimSpace(p.AppSecret) == "" || strings.TrimSpace(code) == "" {
		return ProviderIdentity{}, errors.New("wechat OAuth configuration is incomplete")
	}
	endpoint := p.TokenURL
	if endpoint == "" {
		endpoint = "https://api.weixin.qq.com/sns/oauth2/access_token"
	}
	query := url.Values{}
	query.Set("appid", p.AppID)
	query.Set("secret", p.AppSecret)
	query.Set("code", code)
	query.Set("grant_type", "authorization_code")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return ProviderIdentity{}, fmt.Errorf("create wechat token request: %w", err)
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return ProviderIdentity{}, fmt.Errorf("wechat token request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return ProviderIdentity{}, fmt.Errorf("read wechat token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ProviderIdentity{}, fmt.Errorf("wechat token request returned HTTP %d", response.StatusCode)
	}
	var token weChatTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return ProviderIdentity{}, errors.New("wechat token response is invalid")
	}
	if token.ErrCode != 0 || strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.OpenID) == "" {
		return ProviderIdentity{}, errors.New("wechat authorization was rejected")
	}
	if strings.TrimSpace(token.UnionID) == "" {
		return ProviderIdentity{}, ErrWeChatUnionIDRequired
	}
	return ProviderIdentity{Provider: "wechat_unionid", Subject: strings.TrimSpace(token.UnionID)}, nil
}
