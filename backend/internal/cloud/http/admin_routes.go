package httpapi

import (
	"errors"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/admin"
	"luckytv/backend/internal/cloud/store"
)

type adminRoutes struct {
	service *admin.Service
}

func (a *adminRoutes) login(c *gin.Context) {
	var body struct {
		Email  string `json:"email"`
		Secret string `json:"secret"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.service.Login(body.Email, body.Secret)
	if errors.Is(err, admin.ErrInvalidAdminSession) {
		c.JSON(401, gin.H{"error": "invalid_admin_credentials"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "admin_login_failed"})
		return
	}
	c.JSON(201, result)
}

func (a *adminRoutes) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Admin-Authorization"))
		account, err := a.service.Authenticate(token)
		if errors.Is(err, admin.ErrInvalidAdminSession) {
			c.JSON(401, gin.H{"error": "invalid_admin_session"})
			c.Abort()
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "admin_session_lookup_failed"})
			c.Abort()
			return
		}
		c.Set(adminContextKey, account)
		c.Next()
	}
}

func (a *adminRoutes) account(c *gin.Context) {
	value, ok := c.Get(adminContextKey)
	if !ok {
		c.JSON(401, gin.H{"error": "invalid_admin_session"})
		return
	}
	account, ok := value.(store.AdminAccount)
	if !ok || a.service.RequireRole(account, "accounts.read") != nil {
		c.JSON(403, gin.H{"error": "admin_permission_denied"})
		return
	}
	c.JSON(200, gin.H{"admin": account})
}
