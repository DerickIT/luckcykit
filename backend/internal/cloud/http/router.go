package httpapi

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/admin"
	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/payment"
	"luckytv/backend/internal/cloud/task"
)

type Options struct {
	SandboxEnabled    bool
	Identity          *identity.Service
	Admin             *admin.Service
	EmailOTP          *identity.EmailOTPService
	WeChat            *identity.WeChatProvider
	WeChatCallbackURL string
	HostedModels      *catalog.HostedModelService
	Catalog           *catalog.Service
	Ledger            *ledger.Service
	Payment           *payment.Service
	Tasks             *task.Service
}

const accountContextKey = "cloudAccountID"
const authKindContextKey = "cloudAuthKind"
const adminContextKey = "cloudAdmin"

func NewRouter(options Options) http.Handler {
	auth := &authRoutes{identity: options.Identity, emailOTP: options.EmailOTP, wechat: options.WeChat, wechatCallbackURL: options.WeChatCallbackURL}
	administration := &adminRoutes{service: options.Admin}
	accounts := &accountRoutes{ledger: options.Ledger}
	commerce := &commerceRoutes{catalog: options.Catalog, payment: options.Payment}
	modelTasks := &taskRoutes{tasks: options.Tasks}
	hostedModels := &hostedModelRoutes{models: options.HostedModels}

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "cloud-sandbox"}) })
	v1 := router.Group("/api/v1")
	if options.Admin != nil {
		v1.POST("/admin/login", administration.login)
		adminProtected := v1.Group("/admin")
		adminProtected.Use(administration.requireSession())
		adminProtected.GET("/account", administration.account)
	}
	if options.EmailOTP != nil {
		v1.POST("/auth/email/code", auth.requestEmailCode)
		v1.POST("/auth/email/verify", auth.verifyEmailCode)
	}
	if options.WeChat != nil {
		v1.GET("/auth/wechat/start", auth.beginWeChat)
		v1.GET("/auth/wechat/callback", auth.wechatCallback)
		v1.POST("/auth/wechat/handoff", auth.exchangeHandoff)
	}
	if options.SandboxEnabled {
		v1.GET("/products", commerce.listProducts)
		v1.POST("/sandbox/login", requireSandboxLoopback(), auth.sandboxLogin)
	}
	protected := v1.Group("")
	protected.Use(auth.requireSession())
	protected.POST("/sessions/revoke", auth.revokeSession)
	protected.GET("/ledger", accounts.ledgerEntries)
	protected.GET("/tokens", auth.listAPITokens)
	protected.POST("/tokens", auth.createAPIToken)
	protected.DELETE("/tokens/:tokenID", auth.revokeAPIToken)
	protected.POST("/tokens/revoke-all", auth.revokeAllAPITokens)
	apiAccess := v1.Group("")
	apiAccess.Use(auth.requireSessionOrAPIToken())
	apiAccess.GET("/account", accounts.account)
	if options.HostedModels != nil {
		protected.GET("/models", hostedModels.list)
	}
	protected.GET("/orders", commerce.listOrders)
	protected.POST("/orders", commerce.createOrder)
	protected.GET("/orders/:orderID", commerce.getOrder)
	if options.SandboxEnabled {
		protected.POST("/orders/:orderID/sandbox-payment", requireSandboxLoopback(), commerce.simulatePayment)
	}
	if options.Tasks != nil {
		apiAccess.POST("/model-tasks", modelTasks.submit)
		apiAccess.GET("/model-tasks/:taskID", modelTasks.get)
	}
	router.NoRoute(func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error": "not_found"}) })
	return router
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

func requireSandboxLoopback() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isLoopback(c.Request.RemoteAddr) {
			sandboxLoopbackOnly(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

func sandboxLoopbackOnly(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"error": "sandbox_loopback_only"})
}
