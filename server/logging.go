package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger = zap.NewNop()

func InitLogger(logLevel string) (*zap.Logger, error) {
	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		return nil, fmt.Errorf("unknown log level: %s", logLevel)
	}
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	var err error
	Logger, err = config.Build()
	if err != nil {
		return nil, err
	}
	return Logger, nil
}

func LoggingMiddleware(next http.Handler) http.Handler {
	handler := func(w http.ResponseWriter, r *http.Request) {
		mw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		ctx := r.Context()
		reqId := middleware.GetReqID(ctx)
		t1 := time.Now()
		defer func() {
			Logger.With(zap.String("req_id", reqId)).Info(
				r.URL.Path,
				zap.String("method", r.Method),
				zap.Duration("time_ms", time.Since(t1)),
				zap.Int("status_code", mw.Status()),
				zap.Int("bytes_written", mw.BytesWritten()),
			)
		}()

		next.ServeHTTP(mw, r.WithContext(ctx))
	}
	return http.HandlerFunc(handler)
}

func GetHandlerLoggingFunc() func(*http.Request, error) {
	return func(r *http.Request, err error) {
		if err != nil {
			ctx := r.Context()
			reqId := middleware.GetReqID(ctx)
			Logger.With(zap.String("req_id", reqId)).Error(
				r.URL.Path,
				zap.String("method", r.Method),
				zap.Error(err),
			)
		}
	}
}
