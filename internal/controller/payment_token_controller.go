package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PaymentTokenController struct{ Deps }

type createPaymentTokenBody struct {
	ClientID  int64 `json:"client_id"`
	CompanyID int64 `json:"company_id"`
}

// @Summary Crear token de portal de pago
// @Description Genera el enlace de pago de un cliente. company_id es opcional y, si viene, debe coincidir con el del JWT.
// @Tags Interno
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createPaymentTokenBody true "Cliente"
// @Success 201 {object} model.PaymentToken
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /api/payment-tokens [post]
func (h *PaymentTokenController) Create(c *gin.Context) {
	var body createPaymentTokenBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	companyID := body.CompanyID
	if h.JWTSecret != "" {
		claimCompany := h.companyID(c)
		if companyID == 0 {
			companyID = claimCompany
		} else if companyID != claimCompany {
			c.JSON(http.StatusForbidden, gin.H{"error": "company_id no coincide con el token de sesión"})
			return
		}
	} else if companyID == 0 {
		companyID = h.companyID(c)
	}
	row, err := h.Svc.IssuePaymentToken(c.Request.Context(), companyID, body.ClientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}
