package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"luckytv/backend/internal/cloud/identity"
)

type authRoutes struct {
	identity          *identity.Service
	emailOTP          *identity.EmailOTPService
	wechat            *identity.WeChatProvider
	wechatCallbackURL string
}

func (a *authRoutes) requestEmailCode(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	if err := a.emailOTP.RequestCode(c.Request.Context(), body.Email); err != nil {
		switch {
		case errors.Is(err, identity.ErrEmailOTPUnavailable):
			c.JSON(503, gin.H{"error": "email_verification_unavailable"})
		case errors.Is(err, identity.ErrEmailOTPRateLimited):
			c.JSON(429, gin.H{"error": "verification_rate_limited"})
		case errors.Is(err, identity.ErrEmailOTPInvalid):
			c.JSON(400, gin.H{"error": "invalid_email"})
		default:
			c.JSON(500, gin.H{"error": "verification_request_failed"})
		}
		return
	}
	c.JSON(202, gin.H{"status": "accepted"})
}

func (a *authRoutes) verifyEmailCode(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.emailOTP.VerifyCode(body.Email, body.Code)
	switch {
	case errors.Is(err, identity.ErrEmailOTPInvalid):
		c.JSON(401, gin.H{"error": "invalid_verification_code"})
	case errors.Is(err, identity.ErrEmailOTPUnavailable):
		c.JSON(503, gin.H{"error": "email_verification_unavailable"})
	case err != nil:
		c.JSON(500, gin.H{"error": "email_verification_failed"})
	default:
		c.JSON(201, result)
	}
}

func (a *authRoutes) sandboxLogin(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var body struct {
		Subject string `json:"subject"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.identity.SandboxLogin(c.Request.Context(), body.Subject)
	if errors.Is(err, identity.ErrInvalidSubject) {
		c.JSON(400, gin.H{"error": "invalid_subject"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "login_failed"})
		return
	}
	c.JSON(201, result)
}

func (a *authRoutes) beginWeChat(c *gin.Context) {
	if a.wechat == nil || strings.TrimSpace(a.wechatCallbackURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat_login_unavailable"})
		return
	}
	state, err := a.identity.CreateOAuthStateWithHandoff(10*time.Minute, c.Query("handoffURL"), c.Query("clientState"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_desktop_handoff"})
		return
	}
	authorizeURL, err := a.wechat.AuthorizationURL(state, a.wechatCallbackURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "wechat_login_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorizationURL": authorizeURL, "expiresAt": time.Now().UTC().Add(10 * time.Minute)})
}

func (a *authRoutes) wechatCallback(c *gin.Context) {
	state := c.Query("state")
	if err := a.identity.ConsumeOAuthState(state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_oauth_state"})
		return
	}
	if a.wechat == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat_login_unavailable"})
		return
	}
	providerIdentity, err := a.wechat.Resolve(c.Request.Context(), c.Query("code"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wechat_login_failed"})
		return
	}
	result, err := a.identity.LoginWithIdentity(providerIdentity)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wechat_login_failed"})
		return
	}
	handoff, handoffErr := a.identity.IssueHandoffCode(result.Account.ID, state)
	if handoffErr == nil {
		c.Redirect(http.StatusFound, handoff.URL)
		return
	}
	if !errors.Is(handoffErr, identity.ErrNoDesktopHandoff) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "desktop_handoff_failed"})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "luckytv_cloud_session", Value: result.Token, Path: "/", Expires: result.Expires, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	c.JSON(http.StatusOK, gin.H{"account": result.Account, "expiresAt": result.Expires})
}

func (a *authRoutes) exchangeHandoff(c *gin.Context) {
	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Code) == "" || strings.TrimSpace(body.State) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_handoff"})
		return
	}
	result, err := a.identity.ExchangeHandoffCodeWithState(body.Code, body.State)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_handoff"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *authRoutes) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if strings.HasPrefix(token, identity.APITokenPrefix) {
			c.JSON(http.StatusForbidden, gin.H{"error": "web_session_required"})
			c.Abort()
			return
		}
		if token == "" {
			if cookie, err := c.Request.Cookie("luckytv_cloud_session"); err == nil {
				token = strings.TrimSpace(cookie.Value)
			}
		}
		if token == "" {
			c.JSON(401, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		account, err := a.identity.Authenticate(token)
		if errors.Is(err, identity.ErrInvalidSession) {
			c.JSON(401, gin.H{"error": "invalid_session"})
			c.Abort()
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "session_lookup_failed"})
			c.Abort()
			return
		}
		c.Set(accountContextKey, account.ID)
		c.Set(authKindContextKey, "web_session")
		c.Next()
	}
}

func (a *authRoutes) requireSessionOrAPIToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if !strings.HasPrefix(token, identity.APITokenPrefix) {
			a.requireSession()(c)
			return
		}
		scope := ""
		switch {
		case c.Request.Method == http.MethodGet && c.FullPath() == "/api/v1/account":
			scope = identity.ScopeAccountRead
		case c.Request.Method == http.MethodPost && c.FullPath() == "/api/v1/model-tasks":
			scope = identity.ScopeHostedInvoke
		case c.Request.Method == http.MethodGet && c.FullPath() == "/api/v1/model-tasks/:taskID":
			scope = identity.ScopeHostedTasksRead
		}
		if scope == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "api_token_scope_required"})
			c.Abort()
			return
		}
		auth, err := a.identity.AuthenticateAPIToken(token, scope)
		if errors.Is(err, identity.ErrInvalidAPITokenScope) {
			c.JSON(http.StatusForbidden, gin.H{"error": "api_token_scope_required"})
			c.Abort()
			return
		}
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_token"})
			c.Abort()
			return
		}
		c.Set(accountContextKey, auth.AccountID)
		c.Set(authKindContextKey, "api_token")
		c.Next()
	}
}

func (a *authRoutes) listAPITokens(c *gin.Context) {
	if c.GetString(authKindContextKey) == "api_token" {
		c.JSON(http.StatusForbidden, gin.H{"error": "web_session_required"})
		return
	}
	tokens, err := a.identity.ListAPITokens(c.GetString(accountContextKey))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "api_tokens_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

func (a *authRoutes) createAPIToken(c *gin.Context) {
	if c.GetString(authKindContextKey) == "api_token" {
		c.JSON(http.StatusForbidden, gin.H{"error": "web_session_required"})
		return
	}
	var body struct {
		Name       string   `json:"name"`
		ExpiryDays int      `json:"expiryDays"`
		Scopes     []string `json:"scopes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.identity.CreateAPIToken(c.GetString(accountContextKey), body.Name, body.ExpiryDays, body.Scopes)
	switch {
	case errors.Is(err, identity.ErrInvalidAPITokenName), errors.Is(err, identity.ErrInvalidAPITokenExpiry), errors.Is(err, identity.ErrInvalidAPITokenScope):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_api_token_request"})
	case errors.Is(err, identity.ErrAPITokenLimit):
		c.JSON(http.StatusConflict, gin.H{"error": "api_token_limit_reached"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "api_token_create_failed"})
	default:
		c.JSON(http.StatusCreated, result)
	}
}

func (a *authRoutes) revokeAPIToken(c *gin.Context) {
	if c.GetString(authKindContextKey) == "api_token" {
		c.JSON(http.StatusForbidden, gin.H{"error": "web_session_required"})
		return
	}
	if err := a.identity.RevokeAPIToken(c.GetString(accountContextKey), c.Param("tokenID")); errors.Is(err, identity.ErrInvalidAPIToken) {
		c.JSON(http.StatusNotFound, gin.H{"error": "api_token_not_found"})
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "api_token_revoke_failed"})
	} else {
		c.Status(http.StatusNoContent)
	}
}

func (a *authRoutes) revokeAllAPITokens(c *gin.Context) {
	if c.GetString(authKindContextKey) == "api_token" {
		c.JSON(http.StatusForbidden, gin.H{"error": "web_session_required"})
		return
	}
	if err := a.identity.RevokeAllAPITokens(c.GetString(accountContextKey)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "api_token_revoke_failed"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *authRoutes) revokeSession(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		if cookie, err := c.Request.Cookie("luckytv_cloud_session"); err == nil {
			token = strings.TrimSpace(cookie.Value)
		}
	}
	if token == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if err := a.identity.Revoke(token); errors.Is(err, identity.ErrInvalidSession) {
		c.JSON(401, gin.H{"error": "invalid_session"})
		return
	} else if err != nil {
		c.JSON(500, gin.H{"error": "session_revoke_failed"})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "luckytv_cloud_session", Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	c.Status(204)
}
