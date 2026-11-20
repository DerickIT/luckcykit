package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/admin"
	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/config"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/payment"
	"luckytv/backend/internal/cloud/store"
	"luckytv/backend/internal/cloud/task"
)

type API struct {
	identity *identity.Service
	admin    *admin.Service
	emailOTP *identity.EmailOTPService
	catalog  *catalog.Service
	ledger   *ledger.Service
	payment  *payment.Service
	tasks    *task.Service
	sandbox  bool
}

const accountContextKey = "cloudAccountID"
const adminContextKey = "cloudAdmin"

func NewRouter(cfg config.Config, identities *identity.Service, administrators *admin.Service, emailOTP *identity.EmailOTPService, products *catalog.Service, points *ledger.Service, orders *payment.Service, tasks *task.Service) http.Handler {
	api := &API{identity: identities, admin: administrators, emailOTP: emailOTP, catalog: products, ledger: points, payment: orders, tasks: tasks, sandbox: cfg.SandboxEnabled}
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "cloud-sandbox"}) })
	v1 := router.Group("/api/v1")
	if administrators != nil {
		v1.POST("/admin/login", api.adminLogin)
		adminProtected := v1.Group("/admin")
		adminProtected.Use(api.requireAdminSession())
		adminProtected.GET("/account", api.adminAccount)
	}
	if emailOTP != nil {
		v1.POST("/auth/email/code", api.requestEmailCode)
		v1.POST("/auth/email/verify", api.verifyEmailCode)
	}
	if cfg.SandboxEnabled {
		v1.GET("/products", api.listProducts)
		v1.POST("/sandbox/login", api.sandboxLogin)
	}
	protected := v1.Group("")
	protected.Use(api.requireSession())
	protected.POST("/sessions/revoke", api.revokeSession)
	protected.GET("/account", api.account)
	protected.GET("/ledger", api.ledgerEntries)
	protected.GET("/orders", api.listOrders)
	protected.POST("/orders", api.createOrder)
	protected.GET("/orders/:orderID", api.getOrder)
	if cfg.SandboxEnabled {
		protected.POST("/orders/:orderID/sandbox-payment", api.simulatePayment)
	}
	if tasks != nil {
		protected.POST("/model-tasks", api.submitTask)
		protected.GET("/model-tasks/:taskID", api.getTask)
	}
	router.NoRoute(func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error": "not_found"}) })
	return router
}

func (a *API) adminLogin(c *gin.Context) {
	var body struct {
		Email  string `json:"email"`
		Secret string `json:"secret"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.admin.Login(body.Email, body.Secret)
	if errors.Is(err, admin.ErrInvalidAdminSession) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_admin_credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "admin_login_failed"})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (a *API) submitTask(c *gin.Context) {
	var body struct {
		ModelID         string                 `json:"modelId"`
		Messages        []modelgateway.Message `json:"messages"`
		MaxOutputTokens int64                  `json:"maxOutputTokens"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ModelID) == "" || len(body.Messages) == 0 || body.MaxOutputTokens < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.tasks.Submit(c.Request.Context(), task.SubmitRequest{
		AccountID: c.GetString(accountContextKey), IdempotencyKey: c.GetHeader("Idempotency-Key"), ModelID: body.ModelID,
		Request: modelgateway.Request{Messages: body.Messages, MaxOutputTokens: body.MaxOutputTokens},
	})
	switch {
	case errors.Is(err, task.ErrTaskIdempotency):
		c.JSON(http.StatusConflict, gin.H{"error": "idempotency_key_conflict"})
	case errors.Is(err, task.ErrModelUnavailable):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "model_unavailable"})
	case errors.Is(err, ledger.ErrInsufficientPoints):
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "insufficient_points"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task_submit_failed"})
	default:
		c.JSON(http.StatusAccepted, gin.H{"task": result.Task, "replayed": result.Replayed})
	}
}

func (a *API) getTask(c *gin.Context) {
	result, err := a.tasks.Get(c.GetString(accountContextKey), c.Param("taskID"))
	if errors.Is(err, task.ErrTaskNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "task_not_found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": result})
}

func (a *API) requestEmailCode(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	if err := a.emailOTP.RequestCode(c.Request.Context(), body.Email); err != nil {
		switch {
		case errors.Is(err, identity.ErrEmailOTPUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email_verification_unavailable"})
		case errors.Is(err, identity.ErrEmailOTPRateLimited):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "verification_rate_limited"})
		case errors.Is(err, identity.ErrEmailOTPInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_email"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "verification_request_failed"})
		}
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

func (a *API) verifyEmailCode(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.emailOTP.VerifyCode(body.Email, body.Code)
	switch {
	case errors.Is(err, identity.ErrEmailOTPInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_verification_code"})
	case errors.Is(err, identity.ErrEmailOTPUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email_verification_unavailable"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "email_verification_failed"})
	default:
		c.JSON(http.StatusCreated, result)
	}
}

func (a *API) sandboxLogin(c *gin.Context) {
	if !a.sandbox || !isLoopback(c.Request.RemoteAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sandbox_loopback_only"})
		return
	}
	var body struct {
		Subject string `json:"subject"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := a.identity.SandboxLogin(c.Request.Context(), body.Subject)
	if errors.Is(err, identity.ErrInvalidSubject) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_subject"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login_failed"})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (a *API) requireAdminSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Admin-Authorization"))
		account, err := a.admin.Authenticate(token)
		if errors.Is(err, admin.ErrInvalidAdminSession) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_admin_session"})
			c.Abort()
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "admin_session_lookup_failed"})
			c.Abort()
			return
		}
		c.Set(adminContextKey, account)
		c.Next()
	}
}

func (a *API) adminAccount(c *gin.Context) {
	value, ok := c.Get(adminContextKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_admin_session"})
		return
	}
	account, ok := value.(store.AdminAccount)
	if !ok || a.admin.RequireRole(account, "accounts.read") != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_permission_denied"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"admin": account})
}

func (a *API) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		account, err := a.identity.Authenticate(token)
		if errors.Is(err, identity.ErrInvalidSession) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_session"})
			c.Abort()
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "session_lookup_failed"})
			c.Abort()
			return
		}
		c.Set(accountContextKey, account.ID)
		c.Next()
	}
}

func (a *API) account(c *gin.Context) {
	accountID := c.GetString(accountContextKey)
	balance, err := a.ledger.Balance(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "balance_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accountId": accountID, "points": balance.Points})
}

func (a *API) revokeSession(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := a.identity.Revoke(token); errors.Is(err, identity.ErrInvalidSession) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_session"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session_revoke_failed"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *API) listProducts(c *gin.Context) {
	products, err := a.catalog.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "catalog_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": products})
}

func (a *API) createOrder(c *gin.Context) {
	var body struct {
		ProductID string `json:"productId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ProductID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	order, err := a.payment.CreateOrder(c.GetString(accountContextKey), body.ProductID)
	if errors.Is(err, catalog.ErrProductUnavailable) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "product_unavailable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "order_create_failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"order": order})
}

func (a *API) simulatePayment(c *gin.Context) {
	if !a.sandbox || !isLoopback(c.Request.RemoteAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sandbox_loopback_only"})
		return
	}
	order, replayed, err := a.payment.SimulatePayment(c.GetString(accountContextKey), c.Param("orderID"), c.GetHeader("Idempotency-Key"))
	switch {
	case errors.Is(err, payment.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
	case errors.Is(err, payment.ErrOrderPaid):
		c.JSON(http.StatusConflict, gin.H{"error": "order_already_paid"})
	case errors.Is(err, payment.ErrIdempotencyConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "idempotency_key_conflict"})
	case errors.Is(err, payment.ErrIdempotency):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_idempotency_key"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "payment_failed"})
	default:
		c.JSON(http.StatusOK, gin.H{"order": order, "replayed": replayed})
	}
}

func (a *API) getOrder(c *gin.Context) {
	order, err := a.payment.GetOrder(c.GetString(accountContextKey), c.Param("orderID"))
	if errors.Is(err, payment.ErrOrderNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "order_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": order})
}

func (a *API) listOrders(c *gin.Context) {
	orders, err := a.payment.ListOrders(c.GetString(accountContextKey))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orders_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"orders": orders})
}

func (a *API) ledgerEntries(c *gin.Context) {
	accountID := c.GetString(accountContextKey)
	balance, err := a.ledger.Balance(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ledger_unavailable"})
		return
	}
	entries, err := a.ledger.Entries(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ledger_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"balance": balance.Points, "entries": entries})
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(remoteAddr), "[]")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
