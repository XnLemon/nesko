package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// ListCheckRunsForRef returns one page of GitHub App check runs for a commit,
// branch, or tag reference.
func (c *Client) ListCheckRunsForRef(ctx context.Context, ref RepositoryRef, revision string, query CheckRunQuery) (*gh.ListCheckRunsResults, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	revision = strings.TrimSpace(revision)
	if revision == "" || strings.ContainsAny(revision, "\r\n") {
		return nil, fmt.Errorf("GitHub check reference is required")
	}
	status := strings.ToLower(strings.TrimSpace(query.Status))
	if status != "" && status != "queued" && status != "in_progress" && status != "completed" {
		return nil, fmt.Errorf("unsupported GitHub check status %q", query.Status)
	}
	filter := strings.ToLower(strings.TrimSpace(query.Filter))
	if filter != "" && filter != "latest" && filter != "all" {
		return nil, fmt.Errorf("unsupported GitHub check filter %q", query.Filter)
	}
	options := &gh.ListCheckRunsOptions{ListOptions: *query.Page.listOptions()}
	if name := strings.TrimSpace(query.CheckName); name != "" {
		options.CheckName = &name
	}
	if status != "" {
		options.Status = &status
	}
	if filter != "" {
		options.Filter = &filter
	}
	results, _, err := c.api.Checks.ListCheckRunsForRef(ctx, ref.Owner, ref.Name, revision, options)
	if err != nil {
		return nil, fmt.Errorf("list GitHub checks for %s/%s at %s: %w", ref.Owner, ref.Name, revision, err)
	}
	return results, nil
}

// GetCombinedCommitStatus returns legacy commit statuses aggregated by GitHub.
func (c *Client) GetCombinedCommitStatus(ctx context.Context, ref RepositoryRef, revision string, page PageOptions) (*gh.CombinedStatus, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	revision = strings.TrimSpace(revision)
	if revision == "" || strings.ContainsAny(revision, "\r\n") {
		return nil, fmt.Errorf("GitHub commit reference is required")
	}
	status, _, err := c.api.Repositories.GetCombinedStatus(ctx, ref.Owner, ref.Name, revision, page.listOptions())
	if err != nil {
		return nil, fmt.Errorf("get combined GitHub status for %s/%s at %s: %w", ref.Owner, ref.Name, revision, err)
	}
	return status, nil
}
