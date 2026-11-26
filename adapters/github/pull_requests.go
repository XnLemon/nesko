package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// GetPullRequest loads a pull request by number.
func (c *Client) GetPullRequest(ctx context.Context, ref RepositoryRef, number int) (*gh.PullRequest, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	pullRequest, _, err := c.api.PullRequests.Get(ctx, ref.Owner, ref.Name, number)
	if err != nil {
		return nil, fmt.Errorf("get GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return pullRequest, nil
}

// ListPullRequests returns one page of pull requests.
func (c *Client) ListPullRequests(ctx context.Context, ref RepositoryRef, query PullRequestQuery) ([]*gh.PullRequest, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	state := strings.ToLower(strings.TrimSpace(query.State))
	if state != "" && state != "open" && state != "closed" && state != "all" {
		return nil, fmt.Errorf("unsupported GitHub pull request state %q", query.State)
	}
	pullRequests, _, err := c.api.PullRequests.List(ctx, ref.Owner, ref.Name, &gh.PullRequestListOptions{
		State:       state,
		Head:        strings.TrimSpace(query.Head),
		Base:        strings.TrimSpace(query.Base),
		Sort:        strings.TrimSpace(query.Sort),
		Direction:   strings.TrimSpace(query.Direction),
		ListOptions: *query.Page.listOptions(),
	})
	if err != nil {
		return nil, fmt.Errorf("list GitHub pull requests for %s/%s: %w", ref.Owner, ref.Name, err)
	}
	return pullRequests, nil
}

// CreatePullRequest creates a pull request. The caller selects the head and base.
func (c *Client) CreatePullRequest(ctx context.Context, ref RepositoryRef, input NewPullRequest) (*gh.PullRequest, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Head) == "" || strings.TrimSpace(input.Base) == "" {
		return nil, errors.New("GitHub pull request title, head, and base are required")
	}
	request := gh.CreatePullRequest{
		Title:               &input.Title,
		Head:                strings.TrimSpace(input.Head),
		Base:                strings.TrimSpace(input.Base),
		Body:                &input.Body,
		Draft:               &input.Draft,
		MaintainerCanModify: &input.MaintainerCanModify,
	}
	if headRepository := strings.TrimSpace(input.HeadRepository); headRepository != "" {
		request.HeadRepo = &headRepository
	}
	pullRequest, _, err := c.api.PullRequests.Create(ctx, ref.Owner, ref.Name, request)
	if err != nil {
		return nil, fmt.Errorf("create GitHub pull request for %s/%s: %w", ref.Owner, ref.Name, err)
	}
	return pullRequest, nil
}

// UpdatePullRequest changes the supplied pull request fields.
func (c *Client) UpdatePullRequest(ctx context.Context, ref RepositoryRef, number int, update PullRequestUpdate) (*gh.PullRequest, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	if update.Title == nil && update.Body == nil && update.State == nil && update.MaintainerCanModify == nil {
		return nil, errors.New("GitHub pull request update has no fields")
	}
	if update.State != nil {
		state := strings.ToLower(strings.TrimSpace(*update.State))
		if state != "open" && state != "closed" {
			return nil, fmt.Errorf("unsupported GitHub pull request state %q", *update.State)
		}
		update.State = &state
	}
	pullRequest, _, err := c.api.PullRequests.Edit(ctx, ref.Owner, ref.Name, number, &gh.PullRequest{
		Title:               update.Title,
		Body:                update.Body,
		State:               update.State,
		MaintainerCanModify: update.MaintainerCanModify,
	})
	if err != nil {
		return nil, fmt.Errorf("update GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return pullRequest, nil
}

// ListPullRequestFiles returns one page of changed files.
func (c *Client) ListPullRequestFiles(ctx context.Context, ref RepositoryRef, number int, page PageOptions) ([]*gh.CommitFile, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	files, _, err := c.api.PullRequests.ListFiles(ctx, ref.Owner, ref.Name, number, page.listOptions())
	if err != nil {
		return nil, fmt.Errorf("list files for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return files, nil
}

// ListPullRequestCommits returns one page of commits in a pull request.
func (c *Client) ListPullRequestCommits(ctx context.Context, ref RepositoryRef, number int, page PageOptions) ([]*gh.RepositoryCommit, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	commits, _, err := c.api.PullRequests.ListCommits(ctx, ref.Owner, ref.Name, number, page.listOptions())
	if err != nil {
		return nil, fmt.Errorf("list commits for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return commits, nil
}

// UpdatePullRequestBranch asks GitHub to merge the current base into the pull request branch.
func (c *Client) UpdatePullRequestBranch(ctx context.Context, ref RepositoryRef, number int, expectedHeadSHA string) error {
	if err := validateResource(ref, number); err != nil {
		return err
	}
	expectedHeadSHA = strings.TrimSpace(expectedHeadSHA)
	if expectedHeadSHA == "" {
		return errors.New("expected GitHub pull request head SHA is required")
	}
	request := gh.UpdatePullRequestBranchRequest{ExpectedHeadSHA: &expectedHeadSHA}
	if _, _, err := c.api.PullRequests.UpdateBranch(ctx, ref.Owner, ref.Name, number, request); err != nil {
		return fmt.Errorf("update branch for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return nil
}

// MergePullRequest merges a pull request using the caller's explicit method and expected head.
func (c *Client) MergePullRequest(ctx context.Context, ref RepositoryRef, number int, options MergeOptions) (*gh.PullRequestMergeResult, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	method := strings.ToLower(strings.TrimSpace(options.Method))
	if method != "merge" && method != "squash" && method != "rebase" {
		return nil, fmt.Errorf("unsupported GitHub merge method %q", options.Method)
	}
	expectedHead := strings.TrimSpace(options.ExpectedHead)
	if expectedHead == "" {
		return nil, errors.New("expected GitHub pull request head SHA is required")
	}
	request := &gh.PullRequestOptions{
		MergeMethod: method,
		CommitTitle: strings.TrimSpace(options.CommitTitle),
		SHA:         expectedHead,
	}
	result, _, err := c.api.PullRequests.Merge(ctx, ref.Owner, ref.Name, number, options.CommitMessage, request)
	if err != nil {
		return result, fmt.Errorf("merge GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return result, nil
}
