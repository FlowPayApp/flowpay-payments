package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"

	"github.com/flowpay/flowpay-payments/internal/config"
	"github.com/flowpay/flowpay-payments/internal/controller"
	"github.com/flowpay/flowpay-payments/internal/gateway/transbank"
	"github.com/flowpay/flowpay-payments/internal/middleware"
	"github.com/flowpay/flowpay-payments/internal/repository"
	"github.com/flowpay/flowpay-payments/internal/routes"
	"github.com/flowpay/flowpay-payments/internal/service"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(3 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal("postgres ping:", err)
	}

	repo := repository.New(db)
	var tbk *transbank.Client
	if cfg.TransbankCommerceCode != "" && cfg.TransbankAPIKey != "" {
		tbk = transbank.NewClient(cfg.TransbankEnvironment, cfg.TransbankCommerceCode, cfg.TransbankAPIKey)
	}

	svc := &service.PaymentsService{
		Repo: repo,
		Webpay: &service.WebpayConfig{
			PublicBaseURL:   cfg.PublicBaseURL,
			FrontendBaseURL: cfg.FrontendBaseURL,
			Environment:     cfg.TransbankEnvironment,
			Transbank:       tbk,
		},
	}

	deps := controller.Deps{
		Svc:            svc,
		DefaultCompany: cfg.DefaultCompanyID,
		JWTSecret:      cfg.JWTSecret,
	}

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	routes.Register(r, deps, middleware.BearerJWT(cfg.JWTSecret))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: cfg.Addr, Handler: r}
	go func() {
		printStartup(db, cfg, tbk != nil)
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
		if cfg.PublicBaseURL == "" {
			log.Println("║ Webpay: WARN — configure FLOWPAY_PAYMENTS_PUBLIC_BASE_URL (ngrok)")
		} else {
			log.Println("║ Webpay Plus: activo")
		}
	} else {
		log.Println("║ Webpay: desactivado (sin credenciales Transbank)")
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
