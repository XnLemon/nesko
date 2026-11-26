package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/xnlemon/nesko/adapters/openai"
)

func main() {
	if err := run(); err != nil {
		slog.Error("OpenAI OAuth sample failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := strings.TrimSpace(os.Getenv("NESKO_OPENAI_OAUTH_CREDENTIALS_PATH"))
	if path == "" {
		var err error
		path, err = openai.DefaultCredentialsPath()
		if err != nil {
			return fmt.Errorf("resolve default OpenAI OAuth credentials path: %w", err)
		}
	}
	tokenSource, err := openai.NewOAuthTokenSource(openai.OAuthTokenSourceConfig{CredentialsPath: path})
	if err != nil {
		return fmt.Errorf("configure OpenAI OAuth token source: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := tokenSource.SignIn(ctx, openBrowser); err != nil {
		return fmt.Errorf("sign in with ChatGPT: %w", err)
	}
	slog.Info("OpenAI ChatGPT OAuth credentials saved", "path", path)
	return nil
}

func openBrowser(authorizationURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", authorizationURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", authorizationURL)
	default:
		command = exec.Command("xdg-open", authorizationURL)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start system browser: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release system browser process: %w", err)
	}
	return nil
}
