package httpapi

import (
	"github.com/gin-gonic/gin"

	"luckytv/backend/internal/cloud/ledger"
)

type accountRoutes struct {
	ledger *ledger.Service
}

func (a *accountRoutes) account(c *gin.Context) {
	accountID := c.GetString(accountContextKey)
	balance, err := a.ledger.Balance(accountID)
	if err != nil {
		c.JSON(500, gin.H{"error": "balance_unavailable"})
		return
	}
	c.JSON(200, gin.H{"accountId": accountID, "points": balance.Points})
}

func (a *accountRoutes) ledgerEntries(c *gin.Context) {
	accountID := c.GetString(accountContextKey)
	balance, err := a.ledger.Balance(accountID)
	if err != nil {
		c.JSON(500, gin.H{"error": "ledger_unavailable"})
		return
	}
	entries, err := a.ledger.Entries(accountID)
	if err != nil {
		c.JSON(500, gin.H{"error": "ledger_unavailable"})
		return
	}
	c.JSON(200, gin.H{"balance": balance.Points, "entries": entries})
}
