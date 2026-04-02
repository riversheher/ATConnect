package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

// CallbackResult carries the result of an OAuth callback.
type CallbackResult struct {
	Params map[string][]string
	Err    error
}

// ListenForCallback starts a temporary HTTP server on a random port to receive
// the OAuth callback. It returns the port, a channel that delivers the callback
// result, and a cleanup function to shut down the server.
func ListenForCallback(ctx context.Context) (int, <-chan CallbackResult, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("listening for callback: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	resultCh := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		resultCh <- CallbackResult{Params: r.URL.Query()}
		fmt.Fprint(w, "Authentication successful! You can close this tab.")
	})

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("callback server error", "error", err)
		}
	}()

	cleanup := func() {
		srv.Shutdown(ctx)
	}

	return port, resultCh, cleanup, nil
}
