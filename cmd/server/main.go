package main

import (
	"context"
	"database/sql"
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

	"github.com/flowpay/flowpay-payments/internal/config"
	"github.com/flowpay/flowpay-payments/internal/controller"
	"github.com/flowpay/flowpay-payments/internal/middleware"
	"github.com/flowpay/flowpay-payments/internal/repository"
	"github.com/flowpay/flowpay-payments/internal/revoked"
	"github.com/flowpay/flowpay-payments/internal/routes"
	"github.com/flowpay/flowpay-payments/internal/service"
)

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

	r := gin.Default()
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
	log.Println("flowpay-payments detenido")
}

func printStartup(db *sql.DB, cfg config.Config, webpayOn bool) {
	log.Println("╔══════════════════════════════════════════════════════╗")
	log.Println("║ FlowPay Payments · microservicio de pagos            ║")
	log.Println("╠══════════════════════════════════════════════════════╣")
	log.Printf("║ HTTP: %s", cfg.Addr)
	log.Printf("║ Health: GET %s/health", cfg.Addr)
	log.Printf("║ DB: %s", safeDSN(cfg.DSN))
	if webpayOn {
		log.Printf("║ Webpay: credencial de cada empresa (%s)", cfg.TransbankEnvironment)
	} else {
		log.Println("║ Webpay: sin clave de cifrado; el enlace no cobra con tarjeta")
	}
	if cfg.PublicBaseURL == "" {
		log.Println("║ Webpay: WARN — configure FLOWPAY_PAYMENTS_PUBLIC_BASE_URL")
	}
	for _, t := range []string{"payment_tokens", "payment_transactions", "charges", "payments"} {
		if !tableExists(db, t) {
			log.Printf("║ WARN: falta tabla %s — ejecuta Mysql/postgresql_migration/", t)
		}
	}
	log.Println("╚══════════════════════════════════════════════════════╝")
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
