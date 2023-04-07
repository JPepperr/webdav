package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/file"
	"github.com/heetch/confita/backend/flags"
	"go.uber.org/zap"
	"golang.org/x/net/webdav"
)

var AllWebdavMethods []string = []string{
	"OPTIONS", "GET", "HEAD", "POST", "DELETE", "PUT", "MKCOL",
	"COPY", "MOVE", "LOCK", "UNLOCK", "PROPFIND", "PROPPATCH",
}

type Config struct {
	FileSystemRootPath  string `config:"file-system-path,short=fs,required"`
	Port                uint32 `config:"port"`
	ReadTimeoutSeconds  uint32 `config:"read-timeout-seconds"`
	WriteTimeoutSeconds uint32 `config:"write-timeout-seconds"`
	IdleTimeoutSeconds  uint32 `config:"idle-timeout-seconds"`
	AuthAnyway          bool   `config:"auth-anyway"`
	LogLevel            string `config:"log-level"`
}

func WebDavWrapper(handler *webdav.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, method := range AllWebdavMethods {
				if r.Method == method {
					handler.ServeHTTP(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	cfg := Config{
		Port:                80,
		ReadTimeoutSeconds:  1,
		WriteTimeoutSeconds: 5,
		IdleTimeoutSeconds:  120,
		LogLevel:            "info",
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

	pathRoot := cfg.FileSystemRootPath
	if err := os.Mkdir(pathRoot, os.ModePerm); !os.IsExist(err) {
		logger.Panic("Failed to init file system", zap.Error(err))
	}

	handler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: webdav.Dir(pathRoot),
		LockSystem: webdav.NewMemLS(),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(LoggingMiddleware)
	r.Use(WebDavWrapper(handler))
	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	})

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
