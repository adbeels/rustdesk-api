package http

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// Run sirve la API con timeouts contra conexiones lentas y se apaga en orden con SIGINT/SIGTERM.
// Antes se usaba fvbock/endless (abandonado desde 2017) en Linux y g.Run en Windows.
func Run(g *gin.Engine, addr string) {
	srv := &http.Server{
		Addr:              addr,
		Handler:           g,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		global.Logger.Info("listening on ", addr)
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.Logger.Fatal("server error: ", err)
		}
	case sig := <-quit:
		global.Logger.Info("shutting down: ", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			global.Logger.Error("shutdown: ", err)
		}
	}
}
