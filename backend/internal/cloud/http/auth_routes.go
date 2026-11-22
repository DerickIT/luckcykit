package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/identity"
)

type authRoutes struct {
	identity *identity.Service
	emailOTP *identity.EmailOTPService
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

func (a *authRoutes) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
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
		c.Next()
	}
}

func (a *authRoutes) revokeSession(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
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
	c.Status(204)
}
