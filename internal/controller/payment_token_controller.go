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
