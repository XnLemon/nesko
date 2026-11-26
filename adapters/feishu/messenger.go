package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/xnlemon/nesko/ports"
)

// Messenger replies to Feishu messages using the OpenAPI client.
type Messenger struct {
	client *lark.Client
}

// NewMessenger creates a Feishu message replier.
func NewMessenger(appID, appSecret string) *Messenger {
	client := lark.NewClient(
		appID,
		appSecret,
		lark.WithAppType(larkcore.AppTypeSelfBuilt),
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	return &Messenger{client: client}
}

// ReplyToMessage sends an idempotent text reply to the source message.
func (m *Messenger) ReplyToMessage(ctx context.Context, input ports.ReplyInput) (string, error) {
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

	response, err := m.client.Im.V1.Message.Reply(ctx, request)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", fmt.Errorf("Feishu reply returned an empty response")
	}
	if !response.Success() {
		return "", fmt.Errorf("Feishu reply failed (%d): %s", response.Code, response.Msg)
	}
	if response.Data == nil || response.Data.MessageId == nil {
		return "", nil
	}
	return *response.Data.MessageId, nil
}
