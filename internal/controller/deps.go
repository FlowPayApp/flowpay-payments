package controller

import (
	"strconv"

	"github.com/flowpay/flowpay-payments/internal/service"
	"github.com/gin-gonic/gin"
)

type Deps struct {
	Svc            *service.PaymentsService
	DefaultCompany int64
	JWTSecret      string
}

func (d *Deps) companyID(c *gin.Context) int64 {
	if d.JWTSecret != "" {
		if v, ok := c.Get("company_id"); ok {
			if id, ok := v.(int64); ok {
				return id
			}
		}
		return d.DefaultCompany
	}
	if v := c.Query("company_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	}
	return d.DefaultCompany
}
