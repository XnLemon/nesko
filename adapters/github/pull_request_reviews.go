package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// ListPullRequestReviews returns one page of submitted reviews.
func (c *Client) ListPullRequestReviews(ctx context.Context, ref RepositoryRef, number int, page PageOptions) ([]*gh.PullRequestReview, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	reviews, _, err := c.api.PullRequests.ListReviews(ctx, ref.Owner, ref.Name, number, page.listOptions())
	if err != nil {
		return nil, fmt.Errorf("list reviews for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return reviews, nil
}

// CreatePullRequestReview submits a review or creates a pending review when Event is empty.
func (c *Client) CreatePullRequestReview(ctx context.Context, ref RepositoryRef, number int, input PullRequestReviewInput) (*gh.PullRequestReview, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	event := strings.ToUpper(strings.TrimSpace(input.Event))
	if !validReviewEvent(event, true) {
		return nil, fmt.Errorf("unsupported GitHub review event %q", input.Event)
	}
	if event != "" && strings.TrimSpace(input.Body) == "" {
		return nil, errors.New("GitHub submitted review body is required")
	}
	request, err := reviewRequest(input, event)
	if err != nil {
		return nil, err
	}
	review, _, err := c.api.PullRequests.CreateReview(ctx, ref.Owner, ref.Name, number, request)
	if err != nil {
		return nil, fmt.Errorf("create review for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return review, nil
}

func reviewRequest(input PullRequestReviewInput, event string) (*gh.PullRequestReviewRequest, error) {
	body := input.Body
	request := &gh.PullRequestReviewRequest{Body: &body}
	if commitSHA := strings.TrimSpace(input.CommitSHA); commitSHA != "" {
		request.CommitID = &commitSHA
	}
	if event != "" {
		request.Event = &event
	}
	for _, comment := range input.Comments {
		converted, err := convertReviewComment(comment)
		if err != nil {
			return nil, err
		}
		request.Comments = append(request.Comments, converted)
	}
	return request, nil
}

func convertReviewComment(comment PullRequestReviewCommentInput) (*gh.DraftReviewComment, error) {
	path, body := strings.TrimSpace(comment.Path), strings.TrimSpace(comment.Body)
	if path == "" || body == "" {
		return nil, errors.New("GitHub review comment path and body are required")
	}
	if strings.ContainsAny(path, "\r\n") {
		return nil, errors.New("GitHub review comment path must be a single path")
	}
	if comment.Position > 0 {
		if comment.Line != 0 || comment.Side != "" || comment.StartLine != 0 || comment.StartSide != "" {
			return nil, errors.New("GitHub review comment cannot mix position and line-based coordinates")
		}
		return &gh.DraftReviewComment{Path: &path, Position: &comment.Position, Body: &body}, nil
	}
	if comment.Line < 1 || !validDiffSide(comment.Side) {
		return nil, errors.New("GitHub review comment requires a positive line and LEFT or RIGHT side")
	}
	side := strings.ToUpper(strings.TrimSpace(comment.Side))
	converted := &gh.DraftReviewComment{Path: &path, Body: &body, Line: &comment.Line, Side: &side}
	if comment.StartLine != 0 || strings.TrimSpace(comment.StartSide) != "" {
		if comment.StartLine < 1 || comment.StartLine > comment.Line || !validDiffSide(comment.StartSide) {
			return nil, errors.New("GitHub multiline review comment requires a valid start line and LEFT or RIGHT start side")
		}
		startSide := strings.ToUpper(strings.TrimSpace(comment.StartSide))
		converted.StartLine = &comment.StartLine
		converted.StartSide = &startSide
	}
	return converted, nil
}

func validDiffSide(side string) bool {
	side = strings.ToUpper(strings.TrimSpace(side))
	return side == "LEFT" || side == "RIGHT"
}

func validReviewEvent(event string, allowPending bool) bool {
	return (allowPending && event == "") || event == "APPROVE" || event == "REQUEST_CHANGES" || event == "COMMENT"
}

// ListPullRequestComments returns one page of diff review comments.
func (c *Client) ListPullRequestComments(ctx context.Context, ref RepositoryRef, number int, page PageOptions) ([]*gh.PullRequestComment, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	comments, _, err := c.api.PullRequests.ListComments(ctx, ref.Owner, ref.Name, number, &gh.PullRequestListCommentsOptions{
		ListOptions: *page.listOptions(),
	})
	if err != nil {
		return nil, fmt.Errorf("list review comments for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return comments, nil
}

// ReplyToPullRequestComment replies to an existing diff review comment thread.
func (c *Client) ReplyToPullRequestComment(ctx context.Context, ref RepositoryRef, number int, commentID int64, body string) (*gh.PullRequestComment, error) {
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	if commentID < 1 || strings.TrimSpace(body) == "" {
		return nil, errors.New("GitHub review comment ID and reply body are required")
	}
	comment, _, err := c.api.PullRequests.CreateCommentInReplyTo(ctx, ref.Owner, ref.Name, number, body, commentID)
	if err != nil {
		return nil, fmt.Errorf("reply to review comment on GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return comment, nil
}

// RequestPullRequestReviewers requests reviews from the supplied users or teams.
func (c *Client) RequestPullRequestReviewers(ctx context.Context, ref RepositoryRef, number int, input PullRequestReviewers) (*gh.PullRequest, error) {
	request, err := reviewerRequest(input)
	if err != nil {
		return nil, err
	}
	if err := validateResource(ref, number); err != nil {
		return nil, err
	}
	pullRequest, _, err := c.api.PullRequests.RequestReviewers(ctx, ref.Owner, ref.Name, number, request)
	if err != nil {
		return nil, fmt.Errorf("request reviewers for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return pullRequest, nil
}

// RemovePullRequestReviewers removes the supplied users or teams from a pull request.
func (c *Client) RemovePullRequestReviewers(ctx context.Context, ref RepositoryRef, number int, input PullRequestReviewers) error {
	request, err := reviewerRequest(input)
	if err != nil {
		return err
	}
	if err := validateResource(ref, number); err != nil {
		return err
	}
	if _, err := c.api.PullRequests.RemoveReviewers(ctx, ref.Owner, ref.Name, number, request); err != nil {
		return fmt.Errorf("remove reviewers from GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return nil
}

func reviewerRequest(input PullRequestReviewers) (gh.ReviewersRequest, error) {
	request := gh.ReviewersRequest{
		Reviewers:     compactStrings(input.Users),
		TeamReviewers: compactStrings(input.Teams),
	}
	if len(request.Reviewers) == 0 && len(request.TeamReviewers) == 0 {
		return gh.ReviewersRequest{}, errors.New("at least one GitHub reviewer or team is required")
	}
	return request, nil
}

// UpdatePendingPullRequestReview edits the body of a pending review.
func (c *Client) UpdatePendingPullRequestReview(ctx context.Context, ref RepositoryRef, number int, reviewID int64, body string) (*gh.PullRequestReview, error) {
	if err := validateReview(ref, number, reviewID); err != nil {
		return nil, err
	}
	review, _, err := c.api.PullRequests.UpdateReview(ctx, ref.Owner, ref.Name, number, reviewID, body)
	if err != nil {
		return nil, fmt.Errorf("update pending review for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return review, nil
}

// SubmitPendingPullRequestReview submits an existing pending review.
func (c *Client) SubmitPendingPullRequestReview(ctx context.Context, ref RepositoryRef, number int, input PendingReviewSubmission) (*gh.PullRequestReview, error) {
	if err := validateReview(ref, number, input.ReviewID); err != nil {
		return nil, err
	}
	event := strings.ToUpper(strings.TrimSpace(input.Event))
	if !validReviewEvent(event, false) {
		return nil, fmt.Errorf("unsupported GitHub review event %q", input.Event)
	}
	comment := strings.TrimSpace(input.Body)
	review, _, err := c.api.PullRequests.SubmitReview(ctx, ref.Owner, ref.Name, number, input.ReviewID, gh.PullRequestSubmitReviewRequest{
		Event: event,
		Body:  &comment,
	})
	if err != nil {
		return nil, fmt.Errorf("submit pending review for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return review, nil
}

// DeletePendingPullRequestReview discards an existing pending review.
func (c *Client) DeletePendingPullRequestReview(ctx context.Context, ref RepositoryRef, number int, reviewID int64) error {
	if err := validateReview(ref, number, reviewID); err != nil {
		return err
	}
	if _, _, err := c.api.PullRequests.DeletePendingReview(ctx, ref.Owner, ref.Name, number, reviewID); err != nil {
		return fmt.Errorf("delete pending review for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return nil
}

// DismissPullRequestReview dismisses a submitted review with a caller-provided message.
func (c *Client) DismissPullRequestReview(ctx context.Context, ref RepositoryRef, number int, reviewID int64, message string) (*gh.PullRequestReview, error) {
	if err := validateReview(ref, number, reviewID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("GitHub review dismissal message is required")
	}
	review, _, err := c.api.PullRequests.DismissReview(ctx, ref.Owner, ref.Name, number, reviewID, gh.PullRequestDismissReviewRequest{
		Message: strings.TrimSpace(message),
	})
	if err != nil {
		return nil, fmt.Errorf("dismiss review for GitHub pull request %s/%s#%d: %w", ref.Owner, ref.Name, number, err)
	}
	return review, nil
}

func validateReview(ref RepositoryRef, number int, reviewID int64) error {
	if err := validateResource(ref, number); err != nil {
		return err
	}
	if reviewID < 1 {
		return errors.New("GitHub pull request review ID must be positive")
	}
	return nil
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
