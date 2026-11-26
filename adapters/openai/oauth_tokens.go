package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func (s *OAuthTokenSource) exchangeCode(ctx context.Context, flow *authorizationFlow, clientID string, callback callbackResult) (credentials, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {callback.Code},
		"code_verifier": {flow.verifier},
		"redirect_uri":  {flow.redirectURI},
		"resource":      {openAIResource},
	}
	response, err := s.postTokenForm(ctx, form)
	if err != nil {
		return credentials{}, fmt.Errorf("exchange OpenAI OAuth code: %w", err)
	}
	if err := validateTokenResponse(response, response.Scope, true); err != nil {
		return credentials{}, err
	}
	identity, err := s.verifyIDToken(ctx, response.IDToken, clientID, flow.nonce)
	if err != nil {
		return credentials{}, fmt.Errorf("verify OpenAI OAuth identity: %w", err)
	}
	if flow.previous.Subject != "" && flow.previous.Subject != identity.Subject {
		return credentials{}, errors.New("OpenAI OAuth identity does not match the saved account")
	}
	return credentials{
		Version:      1,
		ClientID:     clientID,
		HostID:       flow.hostID,
		Subject:      identity.Subject,
		Email:        identity.Email,
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		Scopes:       strings.Fields(response.Scope),
		ExpiresAt:    time.Now().Add(time.Duration(response.ExpiresIn) * time.Second),
	}, nil
}

func (s *OAuthTokenSource) refresh(ctx context.Context, current credentials) (credentials, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {current.ClientID},
		"refresh_token": {current.RefreshToken},
		"resource":      {openAIResource},
	}
	response, err := s.postTokenForm(ctx, form)
	if err != nil {
		return credentials{}, fmt.Errorf("refresh OpenAI OAuth token: %w", err)
	}
	scopes := response.Scope
	if scopes == "" {
		scopes = strings.Join(current.Scopes, " ")
	}
	if err := validateTokenResponse(response, scopes, false); err != nil {
		return credentials{}, fmt.Errorf("refresh OpenAI OAuth token: %w", err)
	}
	updated := current
	updated.AccessToken = response.AccessToken
	updated.RefreshToken = response.RefreshToken
	updated.Scopes = strings.Fields(scopes)
	updated.ExpiresAt = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	return updated, nil
}

func (s *OAuthTokenSource) postTokenForm(ctx context.Context, form url.Values) (tokenResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, openAITokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("create OpenAI token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("send OpenAI token request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes+1))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("read OpenAI token response: %w", err)
	}
	if len(body) > maxOAuthResponseBytes {
		return tokenResponse{}, errors.New("OpenAI token response exceeds the size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return tokenResponse{}, fmt.Errorf("OpenAI token endpoint returned HTTP %d: %s", response.StatusCode, compactOAuthError(body))
	}
	var result tokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return tokenResponse{}, fmt.Errorf("decode OpenAI token response: %w", err)
	}
	return result, nil
}

func (s *OAuthTokenSource) verifyIDToken(ctx context.Context, rawToken, clientID, nonce string) (oauthIdentity, error) {
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, s.client), openAIAuthIssuer)
	if err != nil {
		return oauthIdentity{}, fmt.Errorf("discover OpenAI identity provider: %w", err)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: clientID}).Verify(ctx, rawToken)
	if err != nil {
		return oauthIdentity{}, fmt.Errorf("validate ID token: %w", err)
	}
	if idToken.Nonce != nonce {
		return oauthIdentity{}, errors.New("ID token nonce does not match the OAuth request")
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return oauthIdentity{}, fmt.Errorf("decode ID token claims: %w", err)
	}
	if idToken.Subject == "" {
		return oauthIdentity{}, errors.New("ID token subject is empty")
	}
	return oauthIdentity{Subject: idToken.Subject, Email: claims.Email}, nil
}

func validateTokenResponse(response tokenResponse, scope string, requireIDToken bool) error {
	if response.AccessToken == "" || response.RefreshToken == "" || (requireIDToken && response.IDToken == "") {
		return errors.New("OpenAI token response is missing a required token")
	}
	if !strings.EqualFold(response.TokenType, "Bearer") {
		return errors.New("OpenAI token response is not a bearer token")
	}
	if response.ExpiresIn <= 0 || response.ExpiresIn > int64((24*time.Hour)/time.Second) {
		return errors.New("OpenAI token response has an invalid expiry")
	}
	granted := make(map[string]struct{}, len(strings.Fields(scope)))
	for _, value := range strings.Fields(scope) {
		granted[value] = struct{}{}
	}
	for _, required := range requiredScopes {
		if _, ok := granted[required]; !ok {
			return fmt.Errorf("OpenAI OAuth grant is missing required scope %q", required)
		}
	}
	return nil
}

func compactOAuthError(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return parsed.Error
	}
	return "unrecognized OAuth error"
}
