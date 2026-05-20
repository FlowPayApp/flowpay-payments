package routes

import (
	"net/http"

	"github.com/flowpay/flowpay-payments/internal/controller"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, deps controller.Deps, jwtMiddleware gin.HandlerFunc) {
	portal := controller.PortalController{Deps: deps}
	webpay := controller.WebpayController{Deps: deps}
	tokens := controller.PaymentTokenController{Deps: deps}
	payments := controller.PaymentController{Deps: deps}

	r.GET("/api/public/pay/:token", portal.GetPortal)
	r.POST("/api/public/pay/:token/checkout", webpay.Checkout)
	r.POST("/api/public/pay/:token/commit", webpay.Commit)
	r.GET("/api/public/webpay/return/:token", webpay.Return)
	r.POST("/api/public/webpay/return/:token", webpay.Return)
	r.GET("/api/public/webpay/bridge/:id", webpay.Bridge)

	api := r.Group("/api")
	api.Use(jwtMiddleware)
	{
		api.POST("/payment-tokens", tokens.Create)
		api.POST("/payments", payments.RecordManual)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "flowpay-payments"})
	})
}
