package controller

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/service"
	"github.com/gin-gonic/gin"
)

type WebpayController struct{ Deps }

type checkoutBody struct {
	ChargeRefs []string `json:"charge_refs"`
	Method     string   `json:"method"`
}

type commitBody struct {
	TokenWS string `json:"token_ws"`
}

// @Summary Iniciar pago
// @Description method elige la pasarela: webpay (por defecto), khipu o fintoc. Devuelve la URL a la que hay que redirigir al cliente.
// @Tags Portal
// @Accept json
// @Produce json
// @Param token path string true "Token del portal de pago"
// @Param body body checkoutBody true "Cobros a pagar y medio de pago"
// @Success 200 {object} service.WebpayCheckoutResult
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/public/pay/{token}/checkout [post]
func (h *WebpayController) Checkout(c *gin.Context) {
	var body checkoutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	var (
		out *service.WebpayCheckoutResult
		err error
	)
	switch strings.ToLower(strings.TrimSpace(body.Method)) {
	case "khipu":
		out, err = h.Svc.StartKhipuCheckout(c.Request.Context(), c.Param("token"), body.ChargeRefs)
	case "fintoc":
		out, err = h.Svc.StartFintocCheckout(c.Request.Context(), c.Param("token"), body.ChargeRefs)
	default:
		out, err = h.Svc.StartWebpayCheckout(c.Request.Context(), c.Param("token"), body.ChargeRefs)
	}
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary Confirmar pago Webpay
// @Description Confirma la transacción en Transbank (commit). token_ws va en el body o, si falta, en la query.
// @Tags Webpay
// @Accept json
// @Produce json
// @Param token path string true "Token del portal de pago"
// @Param body body commitBody true "Token de Webpay"
// @Param token_ws query string false "Token de Webpay (alternativa al body)"
// @Success 200 {object} service.WebpayCommitResult
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/public/pay/{token}/commit [post]
func (h *WebpayController) Commit(c *gin.Context) {
	var body commitBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	tokenWS := strings.TrimSpace(body.TokenWS)
	if tokenWS == "" {
		tokenWS = strings.TrimSpace(c.Query("token_ws"))
	}
	out, err := h.Svc.CommitWebpayReturn(c.Request.Context(), c.Param("token"), tokenWS)
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary Retorno desde Webpay
// @Description return_url que llama Transbank. Confirma el pago y redirige al front con el resultado. Sin token_ws (pago anulado) redirige como fallido.
// @Tags Webpay
// @Param token path string true "Token del portal de pago"
// @Param token_ws query string false "Token de Webpay"
// @Param TBK_TOKEN query string false "Token cuando el cliente anula el pago"
// @Success 302 "Redirección al front"
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/public/webpay/return/{token} [get]
// @Router /api/public/webpay/return/{token} [post]
func (h *WebpayController) Return(c *gin.Context) {
	portalToken := c.Param("token")
	tokenWS := strings.TrimSpace(c.Query("token_ws"))
	if tokenWS == "" {
		tokenWS = strings.TrimSpace(c.Query("TBK_TOKEN"))
	}
	if tokenWS == "" {
		loc := h.Svc.FrontendReturnURL(portalToken, &service.WebpayCommitResult{
			Status:  "failed",
			Message: "pago cancelado o incompleto",
		})
		c.Redirect(http.StatusFound, loc)
		return
	}
	out, err := h.Svc.CommitWebpayReturn(c.Request.Context(), portalToken, tokenWS)
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, out))
}

// @Summary Puente hacia Webpay
// @Description Página HTML que envía por POST el token_ws al formulario de Transbank.
// @Tags Webpay
// @Produce html
// @Param id path int true "ID de la transacción"
// @Success 200 {string} string "Página HTML"
// @Failure 400 {string} string
// @Router /api/public/webpay/bridge/{id} [get]
func (h *WebpayController) Bridge(c *gin.Context) {
	var txID int64
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &txID); err != nil || txID <= 0 {
		c.String(http.StatusBadRequest, "id inválido")
		return
	}
	actionURL, token, err := h.Svc.WebpayBridge(c.Request.Context(), txID)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	action := html.EscapeString(actionURL)
	tok := html.EscapeString(token)
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>Redirigiendo a Webpay</title></head>
<body>
<p>Redirigiendo a Webpay…</p>
<form id="f" method="post" action="%s"><input type="hidden" name="token_ws" value="%s"/></form>
<script>document.getElementById('f').submit();</script>
</body></html>`, action, tok)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func writeWebpayError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrPaymentTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "token no encontrado"})
	case errors.Is(err, domain.ErrInvalidChargeRefs):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrWebpayNotOffered):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrWebpayCredential):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrWebpayTxNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKhipuNotOffered):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrKhipuCredential):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrFintocNotOffered):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrFintocCredential):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
