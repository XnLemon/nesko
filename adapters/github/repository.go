package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// GetRepository loads repository metadata, including clone URLs and default branch.
func (c *Client) GetRepository(ctx context.Context, ref RepositoryRef) (*gh.Repository, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	repository, _, err := c.api.Repositories.Get(ctx, ref.Owner, ref.Name)
	if err != nil {
		return nil, fmt.Errorf("get GitHub repository %s/%s: %w", ref.Owner, ref.Name, err)
	}
	return repository, nil
}

// GetAuthenticatedUser returns the account represented by the configured credentials.
func (c *Client) GetAuthenticatedUser(ctx context.Context) (*gh.User, error) {
	user, _, err := c.api.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get authenticated GitHub user: %w", err)
	}
	return user, nil
}

// CreateFork requests a fork. GitHub may accept the request before the fork is
// ready; that accepted response is returned as success with repository metadata.
// Callers can poll GetRepository with the returned owner's and name's values.
func (c *Client) CreateFork(ctx context.Context, source RepositoryRef, organization string) (*gh.Repository, error) {
	if err := source.validate(); err != nil {
		return nil, err
	}
	request := gh.CreateForkRequest{}
	if organization = strings.TrimSpace(organization); organization != "" {
		request.Organization = &organization
	}
	repository, _, err := c.api.Repositories.CreateFork(ctx, source.Owner, source.Name, request)
	if err != nil {
		var accepted *gh.AcceptedError
		if errors.As(err, &accepted) {
			return repository, nil
		}
		return repository, fmt.Errorf("create fork of %s/%s: %w", source.Owner, source.Name, err)
	}
	return repository, nil
}
