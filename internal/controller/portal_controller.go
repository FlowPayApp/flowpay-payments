package controller

import (
	"errors"
	"net/http"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/gin-gonic/gin"
)

type PortalController struct{ Deps }

// @Summary Datos del portal de pago
// @Description Cobros pendientes, vencidos y pagados del cliente, más los medios de pago disponibles. Sin login: el token del enlace identifica al cliente.
// @Tags Portal
// @Produce json
// @Param token path string true "Token del portal de pago"
// @Success 200 {object} service.PaymentPortalResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/public/pay/{token} [get]
func (h *PortalController) GetPortal(c *gin.Context) {
	out, err := h.Svc.ResolvePaymentPortal(c.Request.Context(), c.Param("token"))
	if err != nil {
		if errors.Is(err, domain.ErrPaymentTokenNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "token no encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}
