// oramalama-router — always-on request dispatcher daemon.
//
// Usage:
//
//	oramalama-router [--port 8083] [--idle-timeout 10m]
//
// The daemon:
//  1. Starts the small router model (Qwen3-4B) via ramalama if not running.
//  2. Listens on --port for OpenAI-compatible requests.
//  3. For each /v1/chat/completions request, calls the small model with a
//     tool-use prompt to decide which large backend to use.
//  4. Starts the chosen backend on demand (first call: cold start ~15–45 s).
//  5. Proxies the request and response transparently.
//  6. Stops idle backends after --idle-timeout to reclaim VRAM.
//
// Configure OpenCode to use http://127.0.0.1:8083 as its ramalama provider.
// The daemon handles all model-switching automatically — OpenCode never needs
// to know which large model is actually serving the response.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hanthor/oramalama/internal/config"
	"github.com/hanthor/oramalama/internal/router"
)

func main() {
	port := flag.Int("port", router.RouterPort, "port to listen on")
	idle := flag.Duration("idle-timeout", router.IdleTimeout, "stop backends idle for this long")
	flag.Parse()

	cfg := config.Load()

	backends := router.KnownBackends()
	r := router.New(cfg, backends)

	// Override idle timeout if user changed the flag.
	_ = idle // TODO: thread through to RunIdleReaper

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stdout, "oramalama-router: starting small model (%s)...\n", router.RouterModel)
	if err := r.StartRouterModel(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "oramalama-router: small model ready\n")

	r.RunIdleReaper(ctx)

	h := router.NewHandler(r)
	srv := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", *port),
		Handler:      h,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}

	fmt.Fprintf(os.Stdout, "oramalama-router: listening on http://127.0.0.1:%d\n", *port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			cancel()
		}
	}()

	<-ctx.Done()
	fmt.Fprintln(os.Stdout, "\noramalama-router: shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}
