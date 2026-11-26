package feishu

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/xnlemon/nesko"
)

// GatewayConfig contains the Feishu connection and message acceptance settings.
type GatewayConfig struct {
	AppID          string
	AppSecret      string
	BotOpenID      string
	AllowedOpenIDs map[string]struct{}
}

// RequestHandler receives normalized requests from the Feishu gateway.
type RequestHandler interface {
	Handle(ctx context.Context, request nesko.IncomingRequest) error
}

// Gateway listens for Feishu events over the persistent WebSocket connection.
type Gateway struct {
	client *larkws.Client
}

// NewGateway creates the Feishu event client and registers message handling.
func NewGateway(cfg GatewayConfig, handler RequestHandler, logger *slog.Logger) (*Gateway, error) {
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
	cfg.BotOpenID = strings.TrimSpace(cfg.BotOpenID)
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, errors.New("Feishu app ID and app secret are required")
	}
	if handler == nil {
		return nil, errors.New("Feishu request handler is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	allowedOpenIDs := make(map[string]struct{}, len(cfg.AllowedOpenIDs))
	for openID := range cfg.AllowedOpenIDs {
		if openID = strings.TrimSpace(openID); openID != "" {
			allowedOpenIDs[openID] = struct{}{}
		}
	}
	cfg.AllowedOpenIDs = allowedOpenIDs

	eventHandler := dispatcher.NewEventDispatcher("", "")
	eventHandler.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		request, ok := normalizeMessage(event, cfg)
		if !ok {
			return nil
		}
		if err := handler.Handle(ctx, request); err != nil {
			logger.Error("failed to process Feishu message", "message_id", request.MessageID, "error", err)
			return err
		}
		return nil
	})

	client := larkws.NewClient(
		cfg.AppID,
		cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
	)
	client.SetOnReady(func() {
		logger.Info("Feishu long connection is ready")
	})
	client.SetOnReconnecting(func() {
		logger.Info("Feishu long connection is reconnecting")
	})
	client.SetOnReconnected(func() {
		logger.Info("Feishu long connection recovered")
	})
	client.SetOnError(func(err error) {
		logger.Error("Feishu long connection failed", "error", err)
	})

	return &Gateway{client: client}, nil
}

// Run starts the long connection until the context is canceled or it fails.
func (g *Gateway) Run(ctx context.Context) error {
	return g.client.Start(ctx)
}

// Close stops the long connection.
func (g *Gateway) Close() {
	g.client.Close()
}
