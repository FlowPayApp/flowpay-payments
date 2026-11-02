package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/flowpay/flowpay-payments/cmd/server/docs"

	"github.com/flowpay/flowpay-payments/internal/audit"
	"github.com/flowpay/flowpay-payments/internal/config"
	"github.com/flowpay/flowpay-payments/internal/controller"
	"github.com/flowpay/flowpay-payments/internal/middleware"
	"github.com/flowpay/flowpay-payments/internal/repository"
	"github.com/flowpay/flowpay-payments/internal/revoked"
	"github.com/flowpay/flowpay-payments/internal/routes"
	"github.com/flowpay/flowpay-payments/internal/service"
)

// @title FlowPay PAYMENTS API
// @version 1.0
// @description Aplicación de pagos de FlowPay: gestión de pagos, transacciones y notificaciones.
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Token JWT con formato Bearer {token}
func main() {
	cfg := config.Load()
	if config.Production() {
		if len(cfg.JWTSecret) < 16 {
			log.Fatal("FLOWPAY_JWT_SECRET es obligatorio en producción (mín. 16 caracteres, el mismo del SSO)")
		}
		if len(cfg.PaymentCredentialsKey) != 32 {
			log.Fatal("FLOWPAY_PAYMENT_CREDENTIALS_KEY es obligatoria en producción (mín. 32 caracteres, la misma en el backend)")
		}
	}
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(time.Minute)
	db.SetConnMaxLifetime(3 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal("postgres ping:", err)
	}

	repo := repository.New(db)
	if err := repo.EnsureFintocColumns(context.Background()); err != nil {
		log.Printf("warn: columnas de Fintoc: %v", err)
	}
	if err := repo.EnsureAuditEvents(context.Background()); err != nil {
		log.Printf("warn: auditoría: %v", err)
	}
	svc := &service.PaymentsService{
		Repo: repo,
		Webpay: &service.WebpayConfig{
			PublicBaseURL:   cfg.PublicBaseURL,
			FrontendBaseURL: cfg.FrontendBaseURL,
			Environment:     cfg.TransbankEnvironment,
		},
		PaymentKey: cfg.PaymentCredentialsKey,
	}

	deps := controller.Deps{
		Svc:            svc,
		DefaultCompany: cfg.DefaultCompanyID,
		JWTSecret:      cfg.JWTSecret,
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), audit.StampHTTP())
	r.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	r.Use(cors.New(cors.Config{
		AllowOriginFunc:  allowBrowserOrigin,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	revokedList := revoked.New()
	if strings.TrimSpace(cfg.JWTSecret) != "" {
		if err := revoked.Watch(ctx, revokedList, repo.ListLiveRevokedJTIs); err != nil {
			if config.Production() {
				log.Fatalf("revoked_access_tokens: %v", err)
			}
			log.Printf("warn: no se pudo cargar jti revocados: %v", err)
		}
	}
	routes.Register(r, deps, middleware.BearerJWT(cfg.JWTSecret, revokedList.Has))
	if !config.Production() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: r}
	go func() {
		printStartup(db, cfg, len(cfg.PaymentCredentialsKey) == 32)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = db.Close()
	log.Println("servidor detenido")
}

func printStartup(db *sql.DB, cfg config.Config, webpayOn bool) {
	const (
		reset  = "\033[0m"
		bold   = "\033[1m"
		cyan   = "\033[36m"
		green  = "\033[32m"
		yellow = "\033[33m"
	)
	ok := func(v string) string { return green + "OK" + reset + " " + v }
	warn := func(v string) string { return yellow + "WARN" + reset + " " + v }

	log.Println(cyan + "╔══════════════════════════════════════════════════════╗" + reset)
	log.Println(cyan + "║" + reset + " " + bold + "FlowPay Payments · Estado de inicio" + reset + "            " + cyan + "║" + reset)
	log.Println(cyan + "╠══════════════════════════════════════════════════════╣" + reset)
	log.Printf(cyan+"║"+reset+" %s", ok("DB conectada"))
	log.Printf(cyan+"║"+reset+" %s", ok("DB destino: "+safeDSN(cfg.DSN)))
	log.Printf(cyan+"║"+reset+" %s", ok("HTTP listening en "+cfg.Addr))
	log.Printf(cyan+"║"+reset+" %s", ok("Healthcheck: GET "+cfg.Addr+"/health"))
	if webpayOn {
		log.Printf(cyan+"║"+reset+" %s", ok("Webpay: credencial de cada empresa ("+cfg.TransbankEnvironment+")"))
	} else {
		log.Printf(cyan+"║"+reset+" %s", warn("Webpay sin clave de cifrado; el enlace no cobra con tarjeta"))
	}
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		log.Printf(cyan+"║"+reset+" %s", warn("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL vacía"))
	} else {
		log.Printf(cyan+"║"+reset+" %s", ok("URL pública de retornos: "+cfg.PublicBaseURL))
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" {
		log.Printf(cyan+"║"+reset+" %s", warn("FLOWPAY_JWT_SECRET vacío (modo dev)"))
	} else {
		log.Printf(cyan+"║"+reset+" %s", ok("JWT secret cargado"))
	}

	required := []string{"payment_tokens", "payment_transactions", "charges", "payments"}
	var miss []string
	for _, t := range required {
		if !tableExists(db, t) {
			miss = append(miss, t)
		}
	}
	if len(miss) == 0 {
		log.Printf(cyan+"║"+reset+" %s", ok("Tablas críticas en public: "+strings.Join(required, ", ")))
	} else {
		log.Printf(cyan+"║"+reset+" %s", warn("Faltan tablas: "+strings.Join(miss, ", ")))
	}
	log.Printf(cyan+"║"+reset+" %s", fmt.Sprintf("%sAPI base: /api/*%s", bold, reset))
	log.Println(cyan + "╚══════════════════════════════════════════════════════╝" + reset)
}

func tableExists(db *sql.DB, name string) bool {
	var ok bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, name).Scan(&ok)
	return err == nil && ok
}

func allowBrowserOrigin(origin string) bool {
	switch origin {
	case "https://geldflus.com", "https://www.geldflus.com", "https://app.geldflus.com":
		return true
	}
	if !config.Production() && isLocalDevOrigin(origin) {
		return true
	}
	for _, extra := range strings.Split(os.Getenv("FLOWPAY_CORS_ORIGINS"), ",") {
		if strings.TrimSpace(extra) == origin && origin != "" {
			return true
		}
	}
	return false
}

func isLocalDevOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

func safeDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(dsn inválido)"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.Redacted()
}
