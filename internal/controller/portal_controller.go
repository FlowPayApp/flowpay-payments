package controller

import (
	"errors"
	"net/http"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/gin-gonic/gin"
)

type PortalController struct{ Deps }

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
