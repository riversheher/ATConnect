package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

// OIDCServer defines the interface for an OpenID Connect server.
// Currently used for reference and future implementation.
type OIDCServer interface {
	GetJWKS() (string, error)
}

/**
* A quick and minimal callback server to receive the OAuth callback with auth codes.
 */
func listenForCallback(ctx context.Context, callbackChan chan url.Values) (int, *http.Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		callbackChan <- r.URL.Query()
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte("<html><body><h1>Authentication successful! You can close this window.</h1></body></html>"))
		go server.Shutdown(ctx)
	})

	go func() {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	return listener.Addr().(*net.TCPAddr).Port, server, nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Run()
	default: // linux and other unix-like
		return exec.Command("xdg-open", url).Run()
	}
}

/**
* Starts Callback Server for OAuth flow, then initiates the OAuth flow.
 */
func run(ctx context.Context, handle string) error {

	// Accepts a map of URL values as a callback from the OAuth flow. This is where the authorization code or tokens will be received.
	callbackChan := make(chan url.Values, 1)
	port, server, err := listenForCallback(ctx, callbackChan)
	if err != nil {
		return err
	}
	defer server.Close()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Create OAuth client with provided callback URL and in memory storage for tokens.
	config := oauth.NewLocalhostConfig(callbackURL, []string{"atproto"})
	store := oauth.NewMemStore()
	oauthClient := oauth.NewClientApp(&config, store)

	// Start OAuth Flow
	fmt.Printf("Starting OAuth flow for handle: %s...\n", handle)
	authURL, err := oauthClient.StartAuthFlow(ctx, handle)
	if err != nil {
		return fmt.Errorf("failed to start OAuth flow: %w", err)
	}

	// Open the authorization URL in the user's default browser.
	fmt.Printf("Opening Browser... If it doesn't open automatically, please navigate to: %s\n", authURL)
	if !strings.HasPrefix(authURL, "https://") {
		return fmt.Errorf("Authorization URL is non-https, refusing to open browser for security reasons: %s", authURL)
	}

	if err := openBrowser(authURL); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}
	fmt.Println("Waiting for OAuth callback...")
	// Receive the callback from channel we defined earlier. This will block until the callback is received, at which point we can proceed to exchange the authorization code for tokens.
	values := <-callbackChan

	// Exchange the authorization code for session.
	sessionData, err := oauthClient.ProcessCallback(ctx, values)
	if err != nil {
		return fmt.Errorf("failed to process OAuth callback: %w", err)
	}
	fmt.Printf("Logged in! DID: %s\n", sessionData.AccountDID)

	// Resume Session
	session, err := oauthClient.ResumeSession(ctx, sessionData.AccountDID, sessionData.SessionID)
	if err != nil {
		return fmt.Errorf("failed to resume session: %w", err)
	}

	client := session.APIClient()
	var resp struct {
		DID    string `json:"did"`
		Handle string `json:"handle"`
	}
	if err := client.Get(ctx, "com.atproto.server.getSession", nil, &resp); err != nil {
		return fmt.Errorf("failed to get session info: %w", err)
	}

	fmt.Printf("\nSession\n")
	fmt.Printf("DID: %s\n", resp.DID)
	fmt.Printf("Handle: %s\n", resp.Handle)
	fmt.Printf("Host: %s\n", client.Host)

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run main.go <handle>")
		os.Exit(1)
	}

	handle := os.Args[1]

	if err := run(context.Background(), handle); err != nil {
		log.Fatal(err)
	}
}
