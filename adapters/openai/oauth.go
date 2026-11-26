package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	openAIAuthIssuer        = "https://auth.openai.com"
	openAIAuthorizeURL      = "https://auth.openai.com/api/accounts/authorize"
	openAITokenURL          = "https://auth.openai.com/api/accounts/oauth/token"
	openAIResource          = "https://api.openai.com/v1"
	dynamicAgentClientID    = "dynamic_agent_client"
	openAIAgentName         = "Nesko"
	openAICallbackPath      = "/auth/callback"
	maxOAuthResponseBytes   = 1 << 20
	defaultRefreshBefore    = 5 * time.Minute
	defaultOAuthHTTPTimeout = 30 * time.Second
)

var ErrCredentialsNotFound = errors.New("OpenAI ChatGPT OAuth credentials not found")

var requestedScopes = []string{
	"chatgpt.tokens.use.direct",
	"email",
	"offline_access",
	"openid",
	"profile",
	"resource.invoke",
}

var requiredScopes = []string{"chatgpt.tokens.use.direct", "offline_access", "openid", "resource.invoke"}

// OAuthTokenSourceConfig configures official Sign in with ChatGPT credentials.
type OAuthTokenSourceConfig struct {
	CredentialsPath string
	HTTPClient      *http.Client
	RefreshBefore   time.Duration
}

// OAuthTokenSource stores and refreshes Sign in with ChatGPT credentials.
type OAuthTokenSource struct {
	credentialsPath string
	client          *http.Client
	refreshBefore   time.Duration
	gate            chan struct{}
}

type credentials struct {
	Version      int       `json:"version"`
	ClientID     string    `json:"client_id"`
	HostID       string    `json:"ext_agent_host_id"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

type oauthIdentity struct {
	Subject string
	Email   string
}

type authorizationFlow struct {
	previous    credentials
	hostID      string
	redirectURI string
	state       string
	nonce       string
	verifier    string
	listener    net.Listener
}

// NewOAuthTokenSource creates a token source backed by a protected local credential file.
func NewOAuthTokenSource(config OAuthTokenSourceConfig) (*OAuthTokenSource, error) {
	path := strings.TrimSpace(config.CredentialsPath)
	if path == "" {
		var err error
		path, err = DefaultCredentialsPath()
		if err != nil {
			return nil, err
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAI OAuth credentials path: %w", err)
	}
	refreshBefore := config.RefreshBefore
	if refreshBefore < 0 {
		return nil, errors.New("OpenAI OAuth refresh window must not be negative")
	}
	if refreshBefore == 0 {
		refreshBefore = defaultRefreshBefore
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultOAuthHTTPTimeout}
	}
	return &OAuthTokenSource{
		credentialsPath: absolute,
		client:          client,
		refreshBefore:   refreshBefore,
		gate:            make(chan struct{}, 1),
	}, nil
}

// SignIn starts the official ChatGPT plan OAuth flow and asks the caller to open its URL.
func (s *OAuthTokenSource) SignIn(ctx context.Context, openAuthorizationURL func(string) error) error {
	if openAuthorizationURL == nil {
		return errors.New("OpenAI OAuth authorization URL handler is required")
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()

	flow, err := s.prepareAuthorization(ctx)
	if err != nil {
		return err
	}
	result, err := s.waitForCallback(ctx, flow, openAuthorizationURL)
	if err != nil {
		return err
	}
	if result.Issuer != "" && result.Issuer != openAIAuthIssuer {
		return errors.New("OpenAI OAuth callback issuer does not match")
	}
	clientID, err := resolveClientID(flow.previous, result)
	if err != nil {
		return err
	}
	updated, err := s.exchangeCode(ctx, flow, clientID, result)
	if err != nil {
		return err
	}
	return s.storeCredentials(ctx, updated)
}

func (s *OAuthTokenSource) prepareAuthorization(ctx context.Context) (flow *authorizationFlow, returnErr error) {
	previous, err := s.loadAuthorizationCredentials(ctx)
	if err != nil {
		return nil, err
	}
	state, nonce, verifier, err := randomAuthorizationValues()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start OpenAI OAuth callback listener: %w", err)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", listener.Addr().(*net.TCPAddr).Port, openAICallbackPath)
	return &authorizationFlow{
		previous:    previous,
		hostID:      previous.HostID,
		redirectURI: redirectURI,
		state:       state,
		nonce:       nonce,
		verifier:    verifier,
		listener:    listener,
	}, nil
}

func (s *OAuthTokenSource) loadAuthorizationCredentials(ctx context.Context) (previous credentials, returnErr error) {
	fileLock, err := s.lockCredentials(ctx)
	if err != nil {
		return credentials{}, err
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("unlock OpenAI OAuth credentials: %w", err))
		}
	}()

	previous, err = s.readCredentials()
	if errors.Is(err, ErrCredentialsNotFound) {
		hostID, err := randomHostID()
		if err != nil {
			return credentials{}, err
		}
		previous = credentials{Version: 1, HostID: hostID}
		if err := s.writeCredentialFile(previous); err != nil {
			return credentials{}, fmt.Errorf("persist OpenAI OAuth host ID: %w", err)
		}
	} else if err != nil {
		return credentials{}, err
	}
	if !isPendingCredentials(previous) {
		if err := validateStoredCredentials(previous); err != nil {
			return credentials{}, err
		}
	}
	return previous, nil
}

func randomHostID() (string, error) {
	value, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate OpenAI OAuth host ID: %w", err)
	}
	return "urn:uuid:" + value.String(), nil
}

// AccessToken returns an access token with enough remaining lifetime for an API request.
func (s *OAuthTokenSource) AccessToken(ctx context.Context) (token string, returnErr error) {
	if err := s.acquire(ctx); err != nil {
		return "", err
	}
	defer s.release()
	fileLock, err := s.lockCredentials(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("unlock OpenAI OAuth credentials: %w", err))
		}
	}()

	current, err := s.readCredentials()
	if err != nil {
		return "", err
	}
	if err := validateStoredCredentials(current); err != nil {
		return "", err
	}
	if time.Until(current.ExpiresAt) > s.refreshBefore {
		return current.AccessToken, nil
	}
	updated, err := s.refresh(ctx, current)
	if err != nil {
		return "", err
	}
	if err := s.writeCredentials(updated); err != nil {
		return "", err
	}
	return updated.AccessToken, nil
}

func (s *OAuthTokenSource) acquire(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-s.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *OAuthTokenSource) release() {
	<-s.gate
}

var _ interface {
	AccessToken(context.Context) (string, error)
} = (*OAuthTokenSource)(nil)
