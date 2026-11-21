package httpapi

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/payment"
)

type commerceRoutes struct {
	catalog *catalog.Service
	payment *payment.Service
}

func (a *commerceRoutes) listProducts(c *gin.Context) {
	products, err := a.catalog.List()
	if err != nil {
		c.JSON(500, gin.H{"error": "catalog_unavailable"})
		return
	}
	c.JSON(200, gin.H{"products": products})
}

func (a *commerceRoutes) createOrder(c *gin.Context) {
	var body struct {
		ProductID string `json:"productId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ProductID) == "" {
		c.JSON(400, gin.H{"error": "invalid_request"})
		return
	}
	order, err := a.payment.CreateOrder(c.GetString(accountContextKey), body.ProductID)
	if errors.Is(err, catalog.ErrProductUnavailable) {
		c.JSON(422, gin.H{"error": "product_unavailable"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "order_create_failed"})
		return
	}
	c.JSON(201, gin.H{"order": order})
}

func (a *commerceRoutes) simulatePayment(c *gin.Context) {
	order, replayed, err := a.payment.SimulatePayment(c.GetString(accountContextKey), c.Param("orderID"), c.GetHeader("Idempotency-Key"))
	switch {
	case errors.Is(err, payment.ErrOrderNotFound):
		c.JSON(404, gin.H{"error": "order_not_found"})
	case errors.Is(err, payment.ErrOrderPaid):
		c.JSON(409, gin.H{"error": "order_already_paid"})
	case errors.Is(err, payment.ErrIdempotencyConflict):
		c.JSON(409, gin.H{"error": "idempotency_key_conflict"})
	case errors.Is(err, payment.ErrIdempotency):
		c.JSON(400, gin.H{"error": "invalid_idempotency_key"})
	case err != nil:
		c.JSON(500, gin.H{"error": "payment_failed"})
	default:
		c.JSON(200, gin.H{"order": order, "replayed": replayed})
	}
}

func (a *commerceRoutes) getOrder(c *gin.Context) {
	order, err := a.payment.GetOrder(c.GetString(accountContextKey), c.Param("orderID"))
	if errors.Is(err, payment.ErrOrderNotFound) {
		c.JSON(404, gin.H{"error": "order_not_found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "order_unavailable"})
		return
	}
	c.JSON(200, gin.H{"order": order})
}

func (a *commerceRoutes) listOrders(c *gin.Context) {
	orders, err := a.payment.ListOrders(c.GetString(accountContextKey))
	if err != nil {
		c.JSON(500, gin.H{"error": "orders_unavailable"})
		return
	}
	c.JSON(200, gin.H{"orders": orders})
}
