package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/dinalt/clip/handler"
)

const (
	maxWorkersCount = 10
	serveAddr       = ":8080"
)

func main() {
	errC := make(chan error)
	go serveErrors(errC)

	poolC := make(chan struct{}, maxWorkersCount)
	for i := 0; i < maxWorkersCount; i++ {
		poolC <- struct{}{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/clip", handler.New(handler.Params{
		ErrC:  errC,
		PoolC: poolC,
	}))

	srv := http.Server{
		Addr:         serveAddr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: time.Minute,
		IdleTimeout:  5 * time.Second,
		Handler:      mux,
	}

	srvErrC := make(chan error, 1)
	go func() {
		srvErrC <- srv.ListenAndServe()
	}()

	log.Println("listen on", srv.Addr)

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt)

	ctx := context.Background()
	var err error
	select {
	case <-sigC:
		log.Println("shutting down gracefully")
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = srv.Shutdown(ctx)
		cancel()
	case err = <-srvErrC:
	}

	if err != nil {
		log.Fatalf(err.Error())
	}
}

func serveErrors(errC <-chan error) {
	for err := range errC {
		log.Println("handler error:", err)
	}
}
