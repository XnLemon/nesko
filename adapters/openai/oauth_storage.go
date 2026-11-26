package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// DefaultCredentialsPath returns the OS user config path used by OAuthTokenSource.
func DefaultCredentialsPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve default OpenAI OAuth credentials directory: %w", err)
	}
	return filepath.Join(configDirectory, "nesko", "openai-oauth.json"), nil
}

func (s *OAuthTokenSource) lockCredentials(ctx context.Context) (*flock.Flock, error) {
	directory := filepath.Dir(s.credentialsPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create OpenAI OAuth credentials directory: %w", err)
	}
	lock := flock.New(s.credentialsPath + ".lock")
	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("lock OpenAI OAuth credentials: %w", err)
	}
	if !locked {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("lock OpenAI OAuth credentials failed")
	}
	return lock, nil
}

func (s *OAuthTokenSource) storeCredentials(ctx context.Context, value credentials) (returnErr error) {
	fileLock, err := s.lockCredentials(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("unlock OpenAI OAuth credentials: %w", err))
		}
	}()
	return s.writeCredentials(value)
}

func validateStoredCredentials(value credentials) error {
	if value.Version != 1 || value.ClientID == "" || value.HostID == "" || value.Subject == "" || value.AccessToken == "" || value.RefreshToken == "" || value.ExpiresAt.IsZero() {
		return errors.New("OpenAI OAuth credential file is incomplete")
	}
	for _, required := range requiredScopes {
		if !hasScope(value.Scopes, required) {
			return fmt.Errorf("OpenAI OAuth credentials are missing required scope %q", required)
		}
	}
	return nil
}

func isPendingCredentials(value credentials) bool {
	return value.Version == 1 && value.HostID != "" && value.ClientID == "" && value.Subject == "" && value.Email == "" && value.AccessToken == "" && value.RefreshToken == "" && len(value.Scopes) == 0 && value.ExpiresAt.IsZero()
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

func (s *OAuthTokenSource) readCredentials() (credentials, error) {
	file, err := os.Open(s.credentialsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return credentials{}, ErrCredentialsNotFound
		}
		return credentials{}, fmt.Errorf("inspect OpenAI OAuth credentials: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return credentials{}, errors.Join(fmt.Errorf("inspect OpenAI OAuth credentials: %w", err), closeCredentialFile(file, nil))
	}
	if !info.Mode().IsRegular() {
		return credentials{}, closeCredentialFile(file, errors.New("OpenAI OAuth credential path must be a regular file"))
	}
	if info.Mode().Perm()&0o077 != 0 {
		return credentials{}, closeCredentialFile(file, errors.New("OpenAI OAuth credential file permissions must be 0600 or stricter"))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxOAuthResponseBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return credentials{}, errors.Join(wrapError("read OpenAI OAuth credentials", readErr), wrapError("close OpenAI OAuth credentials", closeErr))
	}
	if len(data) > maxOAuthResponseBytes {
		return credentials{}, errors.New("OpenAI OAuth credential file exceeds the size limit")
	}
	var result credentials
	if err := json.Unmarshal(data, &result); err != nil {
		return credentials{}, fmt.Errorf("decode OpenAI OAuth credentials: %w", err)
	}
	return result, nil
}

func wrapError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func (s *OAuthTokenSource) writeCredentials(value credentials) (writeErr error) {
	if err := validateStoredCredentials(value); err != nil {
		return err
	}
	return s.writeCredentialFile(value)
}

func (s *OAuthTokenSource) writeCredentialFile(value credentials) (writeErr error) {
	directory := filepath.Dir(s.credentialsPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create OpenAI OAuth credentials directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".nesko-openai-oauth-*")
	if err != nil {
		return fmt.Errorf("create OpenAI OAuth credentials file: %w", err)
	}
	temporaryPath := file.Name()
	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeErr = errors.Join(writeErr, fmt.Errorf("remove temporary OpenAI OAuth credentials: %w", err))
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return closeCredentialFile(file, fmt.Errorf("secure OpenAI OAuth credentials file: %w", err))
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		return closeCredentialFile(file, fmt.Errorf("encode OpenAI OAuth credentials: %w", err))
	}
	if err := file.Sync(); err != nil {
		return closeCredentialFile(file, fmt.Errorf("sync OpenAI OAuth credentials: %w", err))
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close OpenAI OAuth credentials: %w", err)
	}
	if err := os.Rename(temporaryPath, s.credentialsPath); err != nil {
		return fmt.Errorf("replace OpenAI OAuth credentials: %w", err)
	}
	return nil
}

func closeCredentialFile(file *os.File, cause error) error {
	if err := file.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("close OpenAI OAuth credentials file: %w", err))
	}
	return cause
}
