// Command go-lua-lsp starts the reference LSP adapter over a fresh checker
// WorkspaceSession. It offers stdio by default and plain HTTP POST plus
// long-poll notifications when -http is supplied.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wippyai/go-lua/analysis/check/service"
	"github.com/wippyai/go-lua/analysis/lsp"
)

func main() {
	var httpAddress string
	var debounce time.Duration
	flag.StringVar(&httpAddress, "http", "", "serve JSON-RPC over HTTP on this address")
	flag.DurationVar(&debounce, "debounce", 200*time.Millisecond, "document solve debounce")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := lsp.NewServer(service.NewBatchSession(), lsp.Options{Debounce: debounce})
	if httpAddress != "" {
		httpServer := &http.Server{Addr: httpAddress, Handler: lsp.NewHTTPHandler(server)}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdown)
		}()
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}
	if err := lsp.ServeStdio(ctx, os.Stdin, os.Stdout, server); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
