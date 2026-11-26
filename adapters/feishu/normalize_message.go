package feishu

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/xnlemon/nesko"
)

func normalizeMessage(event *larkim.P2MessageReceiveV1, policy GatewayConfig) (nesko.IncomingRequest, bool) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return nesko.IncomingRequest{}, false
	}
	message := event.Event.Message
	sender := event.Event.Sender
	if value(sender.SenderType) != "user" || message.MessageType == nil || *message.MessageType != "text" {
		return nesko.IncomingRequest{}, false
	}
	if sender.SenderId == nil || sender.SenderId.OpenId == nil || message.MessageId == nil ||
		message.ChatId == nil || message.ChatType == nil || message.Content == nil {
		return nesko.IncomingRequest{}, false
	}

	messageID := strings.TrimSpace(*message.MessageId)
	conversationID := strings.TrimSpace(*message.ChatId)
	requesterID := strings.TrimSpace(*sender.SenderId.OpenId)
	if messageID == "" || conversationID == "" || requesterID == "" || !isSenderAllowed(requesterID, policy.AllowedOpenIDs) {
		return nesko.IncomingRequest{}, false
	}
	if !isConversationAllowed(*message.ChatType, message.Mentions, policy.BotOpenID) {
		return nesko.IncomingRequest{}, false
	}

	text, ok := extractText(*message.Content, message.Mentions)
	if !ok {
		return nesko.IncomingRequest{}, false
	}

	return nesko.IncomingRequest{
		Channel:        nesko.ChannelFeishu,
		MessageID:      messageID,
		ConversationID: conversationID,
		RequesterID:    requesterID,
		Text:           text,
		ReceivedAt:     messageTime(message.CreateTime),
	}, true
}

func isSenderAllowed(senderID string, allowedOpenIDs map[string]struct{}) bool {
	if len(allowedOpenIDs) == 0 {
		return true
	}
	_, allowed := allowedOpenIDs[senderID]
	return allowed
}

func isConversationAllowed(chatType string, mentions []*larkim.MentionEvent, botOpenID string) bool {
	if chatType == "p2p" {
		return true
	}
	if chatType != "group" || botOpenID == "" {
		return false
	}
	for _, mention := range mentions {
		if mention == nil || mention.Id == nil || mention.Id.OpenId == nil {
			continue
		}
		if *mention.Id.OpenId == botOpenID {
			return true
		}
	}
	return false
}

func extractText(content string, mentions []*larkim.MentionEvent) (string, bool) {
	var messageContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &messageContent); err != nil || messageContent.Text == "" {
		return "", false
	}

	text := messageContent.Text
	for _, mention := range mentions {
		if mention != nil && mention.Key != nil && *mention.Key != "" {
			text = strings.ReplaceAll(text, *mention.Key, "")
		}
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func messageTime(timestamp *string) time.Time {
	if timestamp != nil {
		milliseconds, err := strconv.ParseInt(*timestamp, 10, 64)
		if err == nil && milliseconds > 0 {
			return time.UnixMilli(milliseconds).UTC()
		}
	}
	return time.Now().UTC()
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
