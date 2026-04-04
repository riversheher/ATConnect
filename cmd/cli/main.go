package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"runtime"

	"github.com/riversheher/atconnect/internal/config"
	"github.com/riversheher/atconnect/internal/oauth"
	"github.com/riversheher/atconnect/pkg/store/memory"
)

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func run(ctx context.Context, cfg *config.Config) error {
	store := memory.New()

	// Start callback listener on a random port
	port, resultCh, cleanup, err := oauth.ListenForCallback(ctx)
	if err != nil {
		return fmt.Errorf("starting callback listener: %w", err)
	}
	defer cleanup()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	oauthClient := oauth.NewClient(callbackURL, cfg.OAuth.Scopes, store)

	// Prompt for handle
	fmt.Print("Enter your ATProto handle (e.g. user.bsky.social): ")
	var handle string
	fmt.Scanln(&handle)

	// Start OAuth flow
	authURL, err := oauthClient.StartFlow(ctx, handle)
	if err != nil {
		return fmt.Errorf("starting OAuth flow: %w", err)
	}

	fmt.Printf("\nOpening browser to authorize...\nURL: %s\n", authURL)
	if err := openBrowser(authURL); err != nil {
		slog.Warn("could not open browser", "error", err)
		fmt.Println("Please open the URL above in your browser.")
	}

	// Wait for callback
	fmt.Println("\nWaiting for authorization callback...")
	result := <-resultCh
	if result.Err != nil {
		return fmt.Errorf("callback error: %w", result.Err)
	}

	// Process callback
	sess, err := oauthClient.HandleCallback(ctx, url.Values(result.Params))
	if err != nil {
		return fmt.Errorf("processing callback: %w", err)
	}

	fmt.Printf("\nAuthentication successful!\n")
	fmt.Printf("  DID:        %s\n", sess.AccountDID)
	fmt.Printf("  Session ID: %s\n", sess.SessionID)
	fmt.Printf("  PDS Host:   %s\n", sess.HostURL)

	return nil
}

func main() {
	configPath := flag.String("config", "", "Path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set up structured logging
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	if err := run(context.Background(), cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
