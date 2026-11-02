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

// @Summary Registrar pago manual
// @Description Pago recibido fuera de la pasarela (efectivo, transferencia ya recibida) sobre un cobro.
// @Tags Interno
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body recordPaymentBody true "Cobro y monto"
// @Success 201 {object} okResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /api/payments [post]
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
