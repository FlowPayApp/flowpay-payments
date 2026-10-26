package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/service"
	"github.com/gin-gonic/gin"
)

type KhipuController struct{ Deps }

func (h *KhipuController) Return(c *gin.Context) {
	portalToken := c.Param("token")
	if strings.TrimSpace(c.Query("cancel")) == "1" {
		c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, &service.WebpayCommitResult{
			Status:   "failed",
			BuyOrder: strings.TrimSpace(c.Query("orden")),
			Method:   "khipu",
			Message:  "La transferencia fue cancelada",
		}))
		return
	}
	out, err := h.Svc.SyncKhipuReturn(c.Request.Context(), portalToken, c.Query("orden"))
	if err != nil {
		if errors.Is(err, domain.ErrPaymentTokenNotFound) || errors.Is(err, domain.ErrWebpayTxNotFound) {
			writeKhipuError(c, err)
			return
		}
		c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, &service.WebpayCommitResult{
			Status:   "pending",
			BuyOrder: strings.TrimSpace(c.Query("orden")),
			Method:   "khipu",
			Message:  "Khipu está confirmando la transferencia",
		}))
		return
	}
	c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, out))
}

func (h *KhipuController) Status(c *gin.Context) {
	out, err := h.Svc.SyncKhipuReturn(c.Request.Context(), c.Param("token"), c.Query("orden"))
	if err != nil {
		writeKhipuError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *KhipuController) Notify(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	paymentID := khipuPaymentID(raw)
	if paymentID == "" {
		c.Status(http.StatusOK)
		return
	}
	err = h.Svc.NotifyKhipu(c.Request.Context(), paymentID)
	switch {
	case err == nil:
		c.Status(http.StatusOK)
	case errors.Is(err, domain.ErrKhipuPending):
		c.Status(http.StatusServiceUnavailable)
	case errors.Is(err, domain.ErrWebpayTxNotFound), errors.Is(err, domain.ErrKhipuNotOffered), errors.Is(err, domain.ErrKhipuCredential), errors.Is(err, domain.ErrKhipuMismatch):
		log.Printf("[FlowPay Khipu] aviso %s: %v", paymentID, err)
		c.Status(http.StatusOK)
	default:
		log.Printf("[FlowPay Khipu] aviso %s: %v", paymentID, err)
		c.Status(http.StatusServiceUnavailable)
	}
}

func khipuPaymentID(raw []byte) string {
	var payload struct {
		PaymentID string `json:"payment_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.PaymentID)
}

func writeKhipuError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrPaymentTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "token no encontrado"})
	case errors.Is(err, domain.ErrInvalidChargeRefs):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKhipuNotOffered):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKhipuCredential):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKhipuMismatch):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrWebpayTxNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
