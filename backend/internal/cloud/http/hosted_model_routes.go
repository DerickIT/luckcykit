package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/catalog"
)

type hostedModelRoutes struct {
	models *catalog.HostedModelService
}

func (a *hostedModelRoutes) list(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"models": a.models.List()})
}
