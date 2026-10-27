package controller

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/service"
	"github.com/gin-gonic/gin"
)

type FintocController struct{ Deps }

func (h *FintocController) Return(c *gin.Context) {
	portalToken := c.Param("token")
	buyOrder := strings.TrimSpace(c.Query("orden"))
	if strings.TrimSpace(c.Query("cancel")) == "1" {
		c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, &service.WebpayCommitResult{
			Status:   "failed",
			BuyOrder: buyOrder,
			Method:   "fintoc",
			Message:  "La transferencia fue cancelada",
		}))
		return
	}
	out, err := h.Svc.ReadFintocStatus(c.Request.Context(), portalToken, buyOrder)
	if err != nil {
		writeFintocError(c, err)
		return
	}
	c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, out))
}

func (h *FintocController) Status(c *gin.Context) {
	out, err := h.Svc.ReadFintocStatus(c.Request.Context(), c.Param("token"), c.Query("orden"))
	if err != nil {
		writeFintocError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *FintocController) Notify(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	err = h.Svc.NotifyFintoc(c.Request.Context(), c.GetHeader("Fintoc-Signature"), raw, time.Now())
	switch {
	case err == nil:
		c.Status(http.StatusOK)
	case errors.Is(err, domain.ErrFintocSignature):
		log.Printf("[FlowPay Fintoc] aviso con firma inválida")
		c.Status(http.StatusBadRequest)
	case errors.Is(err, domain.ErrFintocMismatch):
		log.Printf("[FlowPay Fintoc] aviso no coincide con el cobro")
		c.Status(http.StatusOK)
	case errors.Is(err, domain.ErrWebpayTxNotFound), errors.Is(err, domain.ErrFintocNotOffered), errors.Is(err, domain.ErrFintocCredential):
		log.Printf("[FlowPay Fintoc] aviso: %v", err)
		c.Status(http.StatusOK)
	default:
		log.Printf("[FlowPay Fintoc] aviso: %v", err)
		c.Status(http.StatusServiceUnavailable)
	}
}

func writeFintocError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrPaymentTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "token no encontrado"})
	case errors.Is(err, domain.ErrFintocNotOffered):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrFintocCredential):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrWebpayTxNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
