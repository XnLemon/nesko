package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// GetIssue loads an issue by number.
func (c *Client) GetIssue(ctx context.Context, ref RepositoryRef, number int) (*gh.Issue, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	issue, _, err := c.api.Issues.Get(ctx, ref.Owner, ref.Name, number)
	if err != nil {
		return nil, fmt.Errorf("get GitHub issue %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return issue, nil
}

// ListIssues returns one page of issues and filters out pull requests.
func (c *Client) ListIssues(ctx context.Context, ref RepositoryRef, query IssueQuery) ([]*gh.Issue, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	state := strings.ToLower(strings.TrimSpace(query.State))
	if state != "" && state != "open" && state != "closed" && state != "all" {
		return nil, fmt.Errorf("unsupported GitHub issue state %q", query.State)
	}
	issues, _, err := c.api.Issues.ListByRepo(ctx, ref.Owner, ref.Name, &gh.IssueListByRepoOptions{
		State:       state,
		Assignee:    strings.TrimSpace(query.Assignee),
		Creator:     strings.TrimSpace(query.Creator),
		Labels:      query.Labels,
		ListOptions: *query.Page.listOptions(),
	})
	if err != nil {
		return nil, fmt.Errorf("list GitHub issues for %s/%s: %w", ref.Owner, ref.Name, err)
	}
	filtered := make([]*gh.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequestLinks == nil {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

// UpdateIssue changes issue fields or its open/closed state.
func (c *Client) UpdateIssue(ctx context.Context, ref RepositoryRef, number int, update gh.UpdateIssueRequest) (*gh.Issue, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	if update.State != nil {
		state := strings.ToLower(strings.TrimSpace(*update.State))
		if state != "open" && state != "closed" {
			return nil, fmt.Errorf("unsupported GitHub issue state %q", *update.State)
		}
		update.State = &state
	}
	issue, _, err := c.api.Issues.Update(ctx, ref.Owner, ref.Name, number, update)
	if err != nil {
		return nil, fmt.Errorf("update GitHub issue %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return issue, nil
}

// ListIssueComments returns one page of comments on an issue or pull request.
func (c *Client) ListIssueComments(ctx context.Context, ref RepositoryRef, number int, page PageOptions) ([]*gh.IssueComment, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	comments, _, err := c.api.Issues.ListComments(ctx, ref.Owner, ref.Name, number, &gh.IssueListCommentsOptions{
		ListOptions: *page.listOptions(),
	})
	if err != nil {
		return nil, fmt.Errorf("list comments for %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return comments, nil
}

// CreateIssueComment posts a comment on an issue or pull request.
func (c *Client) CreateIssueComment(ctx context.Context, ref RepositoryRef, number int, body string) (*gh.IssueComment, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("GitHub issue comment body is required")
	}
	comment, _, err := c.api.Issues.CreateComment(ctx, ref.Owner, ref.Name, number, gh.IssueCommentRequest{Body: body})
	if err != nil {
		return nil, fmt.Errorf("comment on GitHub issue %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return comment, nil
}
