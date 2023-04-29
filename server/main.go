package main

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"sync"
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

var WebdavMethods []string = []string{
	"MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "PROPFIND", "PROPPATCH",
}

var VCWebdavMethods []string = []string{
	"CHECKIN",
}

type Config struct {
	FileSystemRootPath  string `config:"file-system-path,short=fs,required"`
	Port                uint32 `config:"port"`
	ReadTimeoutSeconds  uint32 `config:"read-timeout-seconds"`
	WriteTimeoutSeconds uint32 `config:"write-timeout-seconds"`
	IdleTimeoutSeconds  uint32 `config:"idle-timeout-seconds"`
	LogLevel            string `config:"log-level"`
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

	handlerFSPath, repo, err := InitFs(cfg.FileSystemRootPath, "/root")
	if err != nil {
		logger.Panic("Failed to init file system", zap.Error(err))
	}

	handler := &VCHandler{
		handlerFSPath,
		repo,
		sync.Mutex{},
		webdav.Handler{
			Prefix:     "/",
			FileSystem: webdav.Dir(path.Join(cfg.FileSystemRootPath, handlerFSPath)),
			LockSystem: webdav.NewMemLS(),
			Logger:     GetHandlerLoggingFunc(),
		},
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
