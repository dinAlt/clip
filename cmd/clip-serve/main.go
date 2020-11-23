package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/dinalt/clip/handler"
	"github.com/dinalt/clip/presets"
)

const (
	defaultMaxWorkersCount = 10
	defaultServeAddr       = ":8080"
)

var (
	maxWorkersCountFlag int
	serveAddrFlag       string
	presetsPathFlag     string
)

func init() {
	flag.IntVar(&maxWorkersCountFlag, "w", defaultMaxWorkersCount, "maximum workers count")
	flag.StringVar(&serveAddrFlag, "a", defaultServeAddr, "serve host:port")
	flag.StringVar(&presetsPathFlag, "p", "", "presets json file")
}

func main() {
	flag.Parse()
	poolC := make(chan struct{}, maxWorkersCountFlag)
	for i := 0; i < maxWorkersCountFlag; i++ {
		poolC <- struct{}{}
	}

	var (
		ps  presets.Presets
		err error
	)
	if presetsPathFlag != "" {
		ps, err = presets.FromJSONFile(presetsPathFlag)
		if err != nil {
			log.Fatalf("presets.FromJSONFile: %v", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v0/clip", handler.New(handler.Params{
		PoolC:   poolC,
		Logger:  logger{},
		Presets: ps,
	}))

	srv := http.Server{
		Addr:         serveAddrFlag,
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

type logger struct{}

func (l logger) Error(err error) {
	log.Printf("ERROR: %v", err)
}

func (l logger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
