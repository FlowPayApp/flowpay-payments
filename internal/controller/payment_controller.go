package controller

import (
	"net/http"

	"github.com/flowpay/flowpay-payments/internal/service"
	"github.com/gin-gonic/gin"
)

type PaymentController struct{ Deps }

type recordPaymentBody struct {
	ChargeID int64   `json:"charge_id"`
	Amount   float64 `json:"amount"`
}

func (h *PaymentController) RecordManual(c *gin.Context) {
	var body recordPaymentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if err := h.Svc.RecordManualPayment(c.Request.Context(), h.companyID(c), body.ChargeID, body.Amount); err != nil {
		if service.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}
