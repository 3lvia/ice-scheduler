package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ice-scheduler/scheduler/api/problemdetails"
	"github.com/ice-scheduler/scheduler/internal/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	sloggin "github.com/samber/slog-gin"
)

func Serve(addr string, env runtime.Env) (func(ctx context.Context), <-chan error) {
	appServer := &http.Server{
		Addr:    addr,
		Handler: newHandler(env),
	}

	errChan := make(chan error, 1)

	go func(s *http.Server) {
		err := s.ListenAndServe()
		errChan <- err
	}(appServer)

	slog.Info(fmt.Sprintf("API server is listening on %s", addr))

	return func(ctx context.Context) {
		slog.InfoContext(ctx, "API server is shutting down")
		defer slog.InfoContext(ctx, "API server has shut down")

		if err := appServer.Shutdown(ctx); err != nil {
			_ = appServer.Close()
			slog.Error("could not stop the API server gracefully", "error", err)
		}
	}, errChan
}

func newHandler(env runtime.Env) http.Handler {
	switch env {
	case runtime.Development:
		gin.SetMode(gin.DebugMode)
	case runtime.Test:
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(sloggin.NewWithConfig(slog.Default(), sloggin.Config{
		WithSpanID:  true,
		WithTraceID: true,
		Filters: []sloggin.Filter{
			sloggin.IgnorePath("/metrics"),
			sloggin.IgnorePath("/health"),
		},
	}))
	router.Use(gin.Recovery())

	router.NoRoute(func(c *gin.Context) {
		c.Render(http.StatusNotFound, problemdetails.U(
			http.StatusNotFound,
			"Not Found",
			fmt.Sprintf("path %s not found", c.Request.URL.Path)))
	})

	router.NoMethod(func(c *gin.Context) {
		c.Render(http.StatusMethodNotAllowed, problemdetails.U(
			http.StatusMethodNotAllowed,
			"Method Not Allowed",
			fmt.Sprintf("method %s not allowed", c.Request.Method)))
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/favicon.ico", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusNotFound)
	})

	return router
}