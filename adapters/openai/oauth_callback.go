package openai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type callbackResult struct {
	Code     string
	ClientID string
	Issuer   string
	Err      error
	status   int
	message  string
	complete bool
}

func (s *OAuthTokenSource) waitForCallback(ctx context.Context, flow *authorizationFlow, openAuthorizationURL func(string) error) (result callbackResult, returnErr error) {
	results := make(chan callbackResult, 1)
	server := &http.Server{Handler: s.callbackHandler(flow.state, results)}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(flow.listener) }()
	serveErrorConsumed := false
	defer func() {
		if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			returnErr = errors.Join(returnErr, fmt.Errorf("close OpenAI OAuth callback server: %w", err))
		}
		if !serveErrorConsumed {
			if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
				returnErr = errors.Join(returnErr, fmt.Errorf("serve OpenAI OAuth callback: %w", err))
			}
		}
	}()
	if err := openAuthorizationURL(buildAuthorizationURL(flow)); err != nil {
		return callbackResult{}, fmt.Errorf("open OpenAI authorization page: %w", err)
	}
	select {
	case result := <-results:
		return result, result.Err
	case err := <-serveErrors:
		serveErrorConsumed = true
		return callbackResult{}, fmt.Errorf("serve OpenAI OAuth callback: %w", err)
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	}
}

func (s *OAuthTokenSource) callbackHandler(expectedState string, results chan<- callbackResult) http.Handler {
	var once sync.Once
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		result := parseAuthorizationCallback(request, expectedState)
		if result.status != 0 {
			http.Error(writer, result.message, result.status)
			if result.complete {
				signalCallbackResult(&once, results, result)
			}
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(writer, "Nesko authorization complete. You may close this page.")
		signalCallbackResult(&once, results, result)
	})
}

func signalCallbackResult(once *sync.Once, results chan<- callbackResult, result callbackResult) {
	once.Do(func() { results <- result })
}

func parseAuthorizationCallback(request *http.Request, expectedState string) callbackResult {
	if request.Method != http.MethodGet || request.URL.Path != openAICallbackPath {
		return callbackResult{status: http.StatusNotFound, message: "callback route not found"}
	}
	query := request.URL.Query()
	if query.Get("state") != expectedState {
		return callbackResult{status: http.StatusBadRequest, message: "OAuth state mismatch"}
	}
	if authorizationError := query.Get("error"); authorizationError != "" {
		return callbackResult{
			Err:      fmt.Errorf("OpenAI authorization failed: %s", authorizationError),
			status:   http.StatusBadRequest,
			message:  "OpenAI authorization was not completed",
			complete: true,
		}
	}
	code := query.Get("code")
	if code == "" {
		return callbackResult{status: http.StatusBadRequest, message: "OAuth callback is missing the authorization code"}
	}
	return callbackResult{Code: code, ClientID: query.Get("client_id"), Issuer: query.Get("iss"), complete: true}
}

func buildAuthorizationURL(flow *authorizationFlow) string {
	clientID := flow.previous.ClientID
	if clientID == "" {
		clientID = dynamicAgentClientID
	}
	challenge := sha256.Sum256([]byte(flow.verifier))
	values := url.Values{
		"client_id":             {clientID},
		"ext_agent_host_id":     {flow.hostID},
		"redirect_uri":          {flow.redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(requestedScopes, " ")},
		"resource":              {openAIResource},
		"state":                 {flow.state},
		"nonce":                 {flow.nonce},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
	}
	if flow.previous.ClientID == "" {
		values.Set("agent_name_hint", openAIAgentName)
	}
	return openAIAuthorizeURL + "?" + values.Encode()
}

func randomAuthorizationValues() (state, nonce, verifier string, err error) {
	state, err = randomURLToken(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate OpenAI OAuth state: %w", err)
	}
	nonce, err = randomURLToken(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate OpenAI OAuth nonce: %w", err)
	}
	verifier, err = randomURLToken(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate OpenAI OAuth PKCE verifier: %w", err)
	}
	return state, nonce, verifier, nil
}

func randomURLToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func resolveClientID(previous credentials, callback callbackResult) (string, error) {
	if previous.ClientID != "" {
		if callback.ClientID != "" && callback.ClientID != previous.ClientID {
			return "", errors.New("OpenAI OAuth callback returned a different client ID")
		}
		return previous.ClientID, nil
	}
	if callback.ClientID == "" {
		return "", errors.New("OpenAI OAuth callback did not return a client ID")
	}
	return callback.ClientID, nil
}
