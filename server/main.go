package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/file"
	"github.com/heetch/confita/backend/flags"
	"go.uber.org/zap"
)

var WebdavMethods []string = []string{
	"MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "PROPFIND", "PROPPATCH",
}

var VCWebdavMethods []string = []string{
	MethodVersionControl, MethodCheckout, MethodCheckin, MethodUncheckout,
}

type Config struct {
	FileSystemRootPath  string `config:"file-system-path,short=fs,required"`
	Port                uint32 `config:"port"`
	ReadTimeoutSeconds  uint32 `config:"read-timeout-seconds"`
	WriteTimeoutSeconds uint32 `config:"write-timeout-seconds"`
	IdleTimeoutSeconds  uint32 `config:"idle-timeout-seconds"`
	LogLevel            string `config:"log-level"`
	CacheSize           int    `config:"cache-size"`
}

func main() {
	cfg := Config{
		Port:                80,
		ReadTimeoutSeconds:  1,
		WriteTimeoutSeconds: 5,
		IdleTimeoutSeconds:  120,
		LogLevel:            "info",
		CacheSize:           512,
	}

	err := confita.NewLoader(
		env.NewBackend(),
		flags.NewBackend(),
		file.NewBackend("config.json"),
	).Load(context.Background(), &cfg)
	if err != nil {
		fmt.Println("Failed to read config", err)
		return
	}

	logger, err := InitLogger(cfg.LogLevel)
	if err != nil {
		fmt.Println("Failed to create logger", err)
		return
	}

	handler, err := NewHandler(cfg.FileSystemRootPath, "/root", "/vc_root", cfg.CacheSize)
	if err != nil {
		logger.Panic("Failed to init file system", zap.Error(err))
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(LoggingMiddleware)
	r.Use(middleware.StripSlashes)
	for _, method := range append(WebdavMethods, VCWebdavMethods...) {
		chi.RegisterMethod(method)
		r.Method(method, "/*", handler)
	}
	r.Handle("/*", handler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
		Handler:      r,
	}

	logger.Info("Starting HTTP server", zap.Uint32("addr", cfg.Port))
	err = srv.ListenAndServe()
	logger.Error("Server error", zap.Error(err))
}
