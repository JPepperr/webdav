package webdavvc

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

func GetServer(cfg Config) (*http.Server, error) {
	logger, err := InitLogger(cfg.LogLevel)
	if err != nil {
		fmt.Println("Failed to create logger", err)
		return nil, err
	}

	handler, err := NewHandler(cfg.FileSystemRootPath, "/root", "/vc_root", cfg.CacheSize)
	if err != nil {
		logger.Error("Failed to init file system", zap.Error(err))
		return nil, err
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

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
		Handler:      r,
	}, nil
}
