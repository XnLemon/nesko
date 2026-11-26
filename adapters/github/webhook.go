package github

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

const defaultWebhookMaxPayloadBytes int64 = 25 << 20

// WebhookEvent contains a verified GitHub event and its typed go-github payload.
type WebhookEvent struct {
	DeliveryID string
	Type       string
	Payload    any
}

// WebhookHandler processes a verified event. Delivery deduplication belongs to
// the caller and should use DeliveryID as its idempotency key.
type WebhookHandler func(context.Context, WebhookEvent) error

// WebhookConfig configures a signed GitHub webhook receiver.
type WebhookConfig struct {
	Secret          string
	AllowedEvents   []string
	MaxPayloadBytes int64
	Handler         WebhookHandler
}

// WebhookReceiver validates GitHub webhook signatures and parses event payloads.
type WebhookReceiver struct {
	secret          []byte
	allowedEvents   map[string]struct{}
	maxPayloadBytes int64
	handler         WebhookHandler
}

// NewWebhookReceiver creates a receiver that requires GitHub's SHA-256 signature.
func NewWebhookReceiver(config WebhookConfig) (*WebhookReceiver, error) {
	secret := config.Secret
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("GitHub webhook secret is required")
	}
	if config.Handler == nil {
		return nil, errors.New("GitHub webhook handler is required")
	}
	maxPayloadBytes := config.MaxPayloadBytes
	if maxPayloadBytes == 0 {
		maxPayloadBytes = defaultWebhookMaxPayloadBytes
	}
	if maxPayloadBytes < 1 {
		return nil, errors.New("GitHub webhook maximum payload size must be positive")
	}
	allowedEvents := make(map[string]struct{}, len(config.AllowedEvents))
	for _, eventType := range config.AllowedEvents {
		eventType = strings.ToLower(strings.TrimSpace(eventType))
		if eventType == "" {
			return nil, errors.New("GitHub webhook allowed event names must not be empty")
		}
		allowedEvents[eventType] = struct{}{}
	}
	return &WebhookReceiver{
		secret:          []byte(secret),
		allowedEvents:   allowedEvents,
		maxPayloadBytes: maxPayloadBytes,
		handler:         config.Handler,
	}, nil
}

// ServeHTTP validates and dispatches one GitHub webhook request.
func (receiver *WebhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	event, status, err := receiver.eventFromRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	if err := receiver.handler(r.Context(), event); err != nil {
		http.Error(w, "GitHub webhook processing failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (receiver *WebhookReceiver) eventFromRequest(w http.ResponseWriter, r *http.Request) (WebhookEvent, int, error) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return WebhookEvent{}, http.StatusMethodNotAllowed, errors.New("method not allowed")
	}
	if strings.TrimSpace(r.Header.Get(gh.SHA256SignatureHeader)) == "" {
		return WebhookEvent{}, http.StatusUnauthorized, errors.New("missing GitHub webhook signature")
	}
	eventType, deliveryID := strings.TrimSpace(gh.WebHookType(r)), strings.TrimSpace(gh.DeliveryID(r))
	if eventType == "" || deliveryID == "" {
		return WebhookEvent{}, http.StatusBadRequest, errors.New("missing GitHub webhook event metadata")
	}
	r.Body = http.MaxBytesReader(w, r.Body, receiver.maxPayloadBytes)
	payload, err := gh.ValidatePayload(r, receiver.secret)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return WebhookEvent{}, http.StatusRequestEntityTooLarge, errors.New("GitHub webhook payload too large")
		}
		if strings.Contains(strings.ToLower(err.Error()), "signature") {
			return WebhookEvent{}, http.StatusUnauthorized, errors.New("invalid GitHub webhook signature")
		}
		return WebhookEvent{}, http.StatusBadRequest, errors.New("invalid GitHub webhook payload")
	}
	if len(receiver.allowedEvents) > 0 {
		if _, ok := receiver.allowedEvents[strings.ToLower(eventType)]; !ok {
			return WebhookEvent{}, http.StatusNoContent, nil
		}
	}
	eventPayload, err := gh.ParseWebHook(eventType, payload)
	if err != nil {
		return WebhookEvent{}, http.StatusBadRequest, errors.New("unsupported GitHub webhook event")
	}
	return WebhookEvent{
		DeliveryID: deliveryID,
		Type:       eventType,
		Payload:    eventPayload,
	}, 0, nil
}
