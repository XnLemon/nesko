package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	gh "github.com/google/go-github/v93/github"
)

// ListWorkflowRuns returns one page of Actions runs for a repository.
func (c *Client) ListWorkflowRuns(ctx context.Context, ref RepositoryRef, query WorkflowRunQuery) (*gh.WorkflowRuns, error) {
	if err := ref.validate(); err != nil {
		return nil, err
	}
	options := &gh.ListWorkflowRunsOptions{
		Actor:               strings.TrimSpace(query.Actor),
		Branch:              strings.TrimSpace(query.Branch),
		Event:               strings.TrimSpace(query.Event),
		Status:              strings.TrimSpace(query.Status),
		Created:             strings.TrimSpace(query.Created),
		HeadSHA:             strings.TrimSpace(query.HeadSHA),
		ExcludePullRequests: query.ExcludePullRequests,
		CheckSuiteID:        query.CheckSuiteID,
		ListOptions:         *query.Page.listOptions(),
	}
	runs, _, err := c.api.Actions.ListRepositoryWorkflowRuns(ctx, ref.Owner, ref.Name, options)
	if err != nil {
		return nil, fmt.Errorf("list GitHub Actions runs for %s/%s: %w", ref.Owner, ref.Name, err)
	}
	return runs, nil
}

// GetWorkflowRun loads a workflow run by its repository-local run ID.
func (c *Client) GetWorkflowRun(ctx context.Context, ref RepositoryRef, runID int64) (*gh.WorkflowRun, error) {
	if err := validateRun(ref, runID); err != nil {
		return nil, err
	}
	run, _, err := c.api.Actions.GetWorkflowRunByID(ctx, ref.Owner, ref.Name, runID)
	if err != nil {
		return nil, fmt.Errorf("get GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return run, nil
}

// GetWorkflowRunAttempt loads a specific attempt of a workflow run.
func (c *Client) GetWorkflowRunAttempt(ctx context.Context, ref RepositoryRef, runID int64, attempt int) (*gh.WorkflowRun, error) {
	if err := validateAttempt(ref, runID, attempt); err != nil {
		return nil, err
	}
	run, _, err := c.api.Actions.GetWorkflowRunAttempt(ctx, ref.Owner, ref.Name, runID, attempt, nil)
	if err != nil {
		return nil, fmt.Errorf("get attempt %d for GitHub Actions run %s/%s/%d: %w", attempt, ref.Owner, ref.Name, runID, err)
	}
	return run, nil
}

// ListWorkflowJobs returns one page of jobs for an Actions run.
func (c *Client) ListWorkflowJobs(ctx context.Context, ref RepositoryRef, runID int64, page PageOptions) (*gh.Jobs, error) {
	if err := validateRun(ref, runID); err != nil {
		return nil, err
	}
	jobs, _, err := c.api.Actions.ListWorkflowJobs(ctx, ref.Owner, ref.Name, runID, &gh.ListWorkflowJobsOptions{
		ListOptions: *page.listOptions(),
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs for GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return jobs, nil
}

// ListWorkflowJobsAttempt returns jobs from a specific run attempt.
func (c *Client) ListWorkflowJobsAttempt(ctx context.Context, ref RepositoryRef, runID int64, attempt int, page PageOptions) (*gh.Jobs, error) {
	if err := validateAttempt(ref, runID, attempt); err != nil {
		return nil, err
	}
	jobs, _, err := c.api.Actions.ListWorkflowJobsAttempt(ctx, ref.Owner, ref.Name, runID, int64(attempt), page.listOptions())
	if err != nil {
		return nil, fmt.Errorf("list jobs for attempt %d of GitHub Actions run %s/%s/%d: %w", attempt, ref.Owner, ref.Name, runID, err)
	}
	return jobs, nil
}

// GetWorkflowRunLogsURL returns a short-lived redirect URL for the run logs.
// Treat the URL as a credential and do not write it to logs.
func (c *Client) GetWorkflowRunLogsURL(ctx context.Context, ref RepositoryRef, runID int64) (*url.URL, error) {
	if err := validateRun(ref, runID); err != nil {
		return nil, err
	}
	logsURL, _, err := c.api.Actions.GetWorkflowRunLogs(ctx, ref.Owner, ref.Name, runID, 0)
	if err != nil {
		return nil, fmt.Errorf("get logs URL for GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return logsURL, nil
}

// GetWorkflowRunAttemptLogsURL returns a short-lived URL for one run attempt's logs.
// Treat the URL as a credential and do not write it to logs.
func (c *Client) GetWorkflowRunAttemptLogsURL(ctx context.Context, ref RepositoryRef, runID int64, attempt int) (*url.URL, error) {
	if err := validateAttempt(ref, runID, attempt); err != nil {
		return nil, err
	}
	logsURL, _, err := c.api.Actions.GetWorkflowRunAttemptLogs(ctx, ref.Owner, ref.Name, runID, attempt, 0)
	if err != nil {
		return nil, fmt.Errorf("get logs URL for attempt %d of GitHub Actions run %s/%s/%d: %w", attempt, ref.Owner, ref.Name, runID, err)
	}
	return logsURL, nil
}

// RerunWorkflow restarts all jobs in a workflow run.
func (c *Client) RerunWorkflow(ctx context.Context, ref RepositoryRef, runID int64) error {
	if err := validateRun(ref, runID); err != nil {
		return err
	}
	if _, err := c.api.Actions.RerunWorkflowByID(ctx, ref.Owner, ref.Name, runID); err != nil {
		return fmt.Errorf("rerun GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return nil
}

// RerunFailedWorkflowJobs reruns failed jobs and their dependents.
func (c *Client) RerunFailedWorkflowJobs(ctx context.Context, ref RepositoryRef, runID int64) error {
	if err := validateRun(ref, runID); err != nil {
		return err
	}
	if _, err := c.api.Actions.RerunFailedJobsByID(ctx, ref.Owner, ref.Name, runID); err != nil {
		return fmt.Errorf("rerun failed jobs for GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return nil
}

// RerunWorkflowJob reruns one job and its dependent jobs.
func (c *Client) RerunWorkflowJob(ctx context.Context, ref RepositoryRef, jobID int64) error {
	if err := ref.validate(); err != nil {
		return err
	}
	if jobID < 1 {
		return errors.New("GitHub Actions job ID must be positive")
	}
	if _, err := c.api.Actions.RerunJobByID(ctx, ref.Owner, ref.Name, jobID); err != nil {
		return fmt.Errorf("rerun GitHub Actions job %s/%s/%d: %w", ref.Owner, ref.Name, jobID, err)
	}
	return nil
}

// CancelWorkflowRun cancels a running Actions workflow.
func (c *Client) CancelWorkflowRun(ctx context.Context, ref RepositoryRef, runID int64) error {
	if err := validateRun(ref, runID); err != nil {
		return err
	}
	if _, err := c.api.Actions.CancelWorkflowRunByID(ctx, ref.Owner, ref.Name, runID); err != nil {
		return fmt.Errorf("cancel GitHub Actions run %s/%s/%d: %w", ref.Owner, ref.Name, runID, err)
	}
	return nil
}

// DispatchWorkflowByID starts a workflow_dispatch workflow on the given ref.
func (c *Client) DispatchWorkflowByID(ctx context.Context, ref RepositoryRef, workflowID int64, dispatch WorkflowDispatch) (*gh.WorkflowDispatchRunDetails, error) {
	if err := validateDispatch(ref, dispatch); err != nil {
		return nil, err
	}
	if workflowID < 1 {
		return nil, errors.New("GitHub workflow ID must be positive")
	}
	returnRunDetails := true
	details, _, err := c.api.Actions.CreateWorkflowDispatchEventByID(ctx, ref.Owner, ref.Name, workflowID, gh.CreateWorkflowDispatchEventRequest{
		Ref:              strings.TrimSpace(dispatch.Ref),
		Inputs:           dispatch.Inputs,
		ReturnRunDetails: &returnRunDetails,
	})
	if err != nil {
		return details, fmt.Errorf("dispatch GitHub workflow %s/%s/%d: %w", ref.Owner, ref.Name, workflowID, err)
	}
	return details, nil
}

// DispatchWorkflowByFileName starts a workflow_dispatch workflow by file name.
func (c *Client) DispatchWorkflowByFileName(ctx context.Context, ref RepositoryRef, fileName string, dispatch WorkflowDispatch) (*gh.WorkflowDispatchRunDetails, error) {
	if err := validateDispatch(ref, dispatch); err != nil {
		return nil, err
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || strings.ContainsAny(fileName, "/\\?#%\r\n") || fileName == "." || fileName == ".." {
		return nil, errors.New("GitHub workflow file name must be a simple file name")
	}
	returnRunDetails := true
	details, _, err := c.api.Actions.CreateWorkflowDispatchEventByFileName(ctx, ref.Owner, ref.Name, fileName, gh.CreateWorkflowDispatchEventRequest{
		Ref:              strings.TrimSpace(dispatch.Ref),
		Inputs:           dispatch.Inputs,
		ReturnRunDetails: &returnRunDetails,
	})
	if err != nil {
		return details, fmt.Errorf("dispatch GitHub workflow %s/%s/%s: %w", ref.Owner, ref.Name, fileName, err)
	}
	return details, nil
}

func validateDispatch(ref RepositoryRef, dispatch WorkflowDispatch) error {
	if err := ref.validate(); err != nil {
		return err
	}
	if dispatch.Ref = strings.TrimSpace(dispatch.Ref); dispatch.Ref == "" || strings.ContainsAny(dispatch.Ref, "\r\n") {
		return errors.New("GitHub workflow dispatch ref is required")
	}
	if len(dispatch.Inputs) > 25 {
		return errors.New("GitHub workflow dispatch supports at most 25 inputs")
	}
	return nil
}

func validateRun(ref RepositoryRef, runID int64) error {
	if err := ref.validate(); err != nil {
		return err
	}
	if runID < 1 {
		return errors.New("GitHub Actions run ID must be positive")
	}
	return nil
}

func validateAttempt(ref RepositoryRef, runID int64, attempt int) error {
	if err := validateRun(ref, runID); err != nil {
		return err
	}
	if attempt < 1 {
		return errors.New("GitHub Actions attempt number must be positive")
	}
	return nil
}
