package main

import (
	"context"
	"fmt"

	webdavvc "jpepper_webdav/webdavvc/lib"

	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/file"
	"github.com/heetch/confita/backend/flags"
	"go.uber.org/zap"
)

func main() {
	cfg := webdavvc.Config{
		FileSystemRootPath:  "./webdav_fs",
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

	srv, err := webdavvc.GetServer(cfg)
	if err != nil {
		panic(err)
	}

	webdavvc.Logger.Info("Starting HTTP server", zap.Uint32("addr", cfg.Port))
	err = srv.ListenAndServe()
	webdavvc.Logger.Error("Server error", zap.Error(err))
}
