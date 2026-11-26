package nesko

import "time"

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
