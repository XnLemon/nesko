package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/ports"
)

// Messenger replies to Feishu messages using the OpenAPI client.
type Messenger struct {
	client *lark.Client
}

// NewMessenger creates a Feishu message replier.
func NewMessenger(appID, appSecret string) (*Messenger, error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return nil, errors.New("feishu app ID and app secret are required")
	}
	client := lark.NewClient(
		appID,
		appSecret,
		lark.WithAppType(larkcore.AppTypeSelfBuilt),
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	return &Messenger{client: client}, nil
}

// ReplyToMessage sends an idempotent text reply to the source message.
func (m *Messenger) ReplyToMessage(ctx context.Context, input ports.ReplyInput) (string, error) {
	if input.Channel != nesko.ChannelFeishu || strings.TrimSpace(input.MessageID) == "" ||
		strings.TrimSpace(input.Text) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return "", errors.New("feishu reply requires a Feishu channel, message ID, text, and idempotency key")
	}
	content, err := json.Marshal(map[string]string{"text": input.Text})
	if err != nil {
		return "", fmt.Errorf("encode Feishu reply: %w", err)
	}
	request := larkim.NewReplyMessageReqBuilder().
		MessageId(input.MessageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			Content(string(content)).
			MsgType("text").
			Uuid(input.IdempotencyKey).
			Build()).
		Build()

	response, err := m.client.Im.Message.Reply(ctx, request)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", fmt.Errorf("feishu reply returned an empty response")
	}
	if !response.Success() {
		return "", fmt.Errorf("feishu reply failed (%d): %s", response.Code, response.Msg)
	}
	if response.Data == nil || response.Data.MessageId == nil || strings.TrimSpace(*response.Data.MessageId) == "" {
		return "", fmt.Errorf("feishu reply returned no reply message ID")
	}
	return *response.Data.MessageId, nil
}
