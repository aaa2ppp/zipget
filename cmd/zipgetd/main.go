package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zipget/internal/api"
	"zipget/internal/config"
	"zipget/internal/loader"
	"zipget/internal/logger"
	"zipget/internal/manager"
	"zipget/internal/memstor"
	"zipget/internal/protect"

	"github.com/joho/godotenv"
)

const (
	shutdownTimeout = 30 * time.Second
	apiBasePath     = "/api"
	filesBasePath   = "/files"
)

func main() {
	godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	logger.SetupDefault(cfg.Logger)

	slog.Debug("server config", "cfg", cfg)

	client := newHTTPClient()
	stor := memstor.New(memstor.Config{
		MaxTotal: cfg.Manager.MaxTotal,
		MaxFiles: cfg.Manager.MaxFiles,
		TaskTTL:  cfg.Manager.TaskTTL,
	})
	defer stor.Cancel()
	loader := loader.New(client, cfg.Loader.AllowMIMETypes)
	manager := manager.New(cfg.Manager, stor, loader)

	handler := logger.HTTPLogging(slog.Default(), api.New(manager, apiBasePath, filesBasePath))
	server := newServer(cfg.Server.Addr, handler)

	done := make(chan int)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		s := <-c
		slog.Info("shutdown by signal", "signal", s.String())

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("sutdown failed", "error", err)
		}

		close(done)
	}()

	slog.Info("server startup", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}

	os.Exit(<-done)
}

// newHTTPClient создаёт клиент с разумными таймаутами для загрузки файлов и защитой от SSRF.
func newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Client{
		Transport: &http.Transport{
			// SSRF protect
			// FIXME: это решение "на коленке"
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				addr, err := protect.ReplaceHostToIP(addr)
				if err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, addr)
			},
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// newServer создаёт HTTP-сервер с разумными таймаутами для потоковой загрузки.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,

		// Таймауты на уровне соединения
		ReadTimeout:       5 * time.Second, // сколько времени даём клиенту на отправку запроса
		ReadHeaderTimeout: 3 * time.Second, // сколько ждём только заголовки
		WriteTimeout:      5 * time.Minute, // сколько времени даём на отправку ответа (важно для потоковой загрузки!)
		IdleTimeout:       1 * time.Minute, // для keep-alive соединений

		// Ограничение на размер заголовков
		MaxHeaderBytes: 8192, // 8 KB
	}
}
