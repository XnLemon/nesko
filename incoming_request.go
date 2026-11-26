package nesko

import (
	"errors"
	"strings"
	"time"
)

// ChannelFeishu identifies requests received through Feishu.
const ChannelFeishu = "feishu"

// IncomingRequest is the normalized request passed from a channel adapter.
type IncomingRequest struct {
	Channel        string
	MessageID      string
	ConversationID string
	RequesterID    string
	Text           string
	ReceivedAt     time.Time
}

// Validate checks that the request can be routed and deduplicated safely.
func (r IncomingRequest) Validate() error {
	if strings.TrimSpace(r.Channel) == "" || strings.TrimSpace(r.MessageID) == "" ||
		strings.TrimSpace(r.ConversationID) == "" || strings.TrimSpace(r.RequesterID) == "" ||
		strings.TrimSpace(r.Text) == "" {
		return errors.New("incoming request requires channel, message, conversation, requester, and text")
	}
	return nil
}
