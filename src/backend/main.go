package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/queue"
	appRuntime "github.com/chenpy/ps5-ftp-fnos/src/backend/internal/runtime"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/web"
)

func main() {
	cfg := appRuntime.Load()
	if err := cfg.Prepare(); err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(cfg.DBPath, cfg.KeyPath)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if err = s.InterruptInFlight(context.Background()); err != nil {
		log.Fatal(err)
	}
	lib, err := library.New(context.Background(), s)
	if err != nil {
		log.Fatal(err)
	}
	tasks := queue.New(s, lib)
	tasks.Start()
	defer tasks.Close()
	handler := web.New(s, lib, tasks, cfg.UIDir)
	server := &http.Server{Addr: cfg.Listen + ":" + cfg.Port, Handler: handler.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	errs := make(chan error, 1)
	go func() { log.Printf("PS5 FTP Manager listening on %s", server.Addr); errs <- server.ListenAndServe() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}
