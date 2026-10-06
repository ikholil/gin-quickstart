package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gin-quickstart/internal/config"
	"gin-quickstart/internal/database"
	"gin-quickstart/internal/httpapi"
	"gin-quickstart/internal/repository"
	"gin-quickstart/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	db, err := database.Open(startupCtx, cfg.DBPath)
	cancelStartup()
	if err != nil {
		return err
	}
	defer db.Close()

	repo := repository.New(db)
	appService := service.New(repo, cfg.JWTSecret, cfg.JWTTTL)
	if err := appService.EnsureAdmin(context.Background(), cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("provision initial admin: %w", err)
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpapi.New(appService).Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	log.Printf("ecommerce API listening on :%d", cfg.Port)

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	}
}
