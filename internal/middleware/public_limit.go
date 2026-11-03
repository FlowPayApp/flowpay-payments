package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/flowpay/flowpay-payments/internal/audit"
	"github.com/flowpay/flowpay-payments/internal/ratelimit"
	"github.com/gin-gonic/gin"
)

// Un pago abre el portal, vuelve de Webpay y puede recargar. Eso son unos pocos
// requests. 60 por minuto y por IP deja pasar ese uso y responde 429 a una ráfaga.
const (
	publicPayMax    = 60
	publicPayWindow = time.Minute
)

// PublicPaymentLimit cuenta el portal de pago y el retorno de Webpay por IP.
// El mismo handler debe usarse en las dos rutas para que compartan el conteo.
// note se llama una vez por IP cuando esa IP empieza a recibir 429.
func PublicPaymentLimit(note func(*gin.Context)) gin.HandlerFunc {
	lim := ratelimit.New()
	return func(c *gin.Context) {
		ip := audit.RequestIP(c)
		if ip == "" {
			ip = "unknown"
		}
		now := time.Now()
		if lim.Blocked(ip, publicPayMax, publicPayWindow, now) {
			if audit.Once("public-pay-limited:"+ip, publicPayWindow) {
				log.Printf("tope de pago público por IP alcanzado ip=%s", ip)
				if note != nil {
					note(c)
				}
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "demasiadas solicitudes"})
			return
		}
		_, _ = lim.Hit(ip, publicPayMax, publicPayWindow, now)
		c.Next()
	}
}
