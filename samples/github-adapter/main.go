package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	ghadapter "github.com/xnlemon/nesko/adapters/github"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		return fmt.Errorf("usage: go run ./samples/github-adapter owner/repository [pull-request-number]")
	}
	owner, repository, ok := strings.Cut(os.Args[1], "/")
	if !ok || owner == "" || repository == "" || strings.Contains(repository, "/") {
		return fmt.Errorf("repository must use the owner/repository format")
	}
	client, err := ghadapter.NewClient(ghadapter.Config{
		Token: firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")),
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	ref := ghadapter.RepositoryRef{Owner: owner, Name: repository}
	if err := inspectRepository(ctx, client, ref); err != nil {
		return err
	}
	if len(os.Args) == 2 {
		return nil
	}
	pullNumber, err := strconv.Atoi(os.Args[2])
	if err != nil || pullNumber < 1 {
		return fmt.Errorf("pull request number must be positive")
	}
	return inspectPullRequest(ctx, client, ref, pullNumber)
}

func inspectRepository(ctx context.Context, client *ghadapter.Client, ref ghadapter.RepositoryRef) error {
	metadata, err := client.GetRepository(ctx, ref)
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"repository":     metadata.GetFullName(),
		"url":            metadata.GetHTMLURL(),
		"default_branch": metadata.GetDefaultBranch(),
		"clone_url":      metadata.GetCloneURL(),
		"ssh_url":        metadata.GetSSHURL(),
	})
}

func inspectPullRequest(ctx context.Context, client *ghadapter.Client, ref ghadapter.RepositoryRef, pullNumber int) error {
	pullRequest, err := client.GetPullRequest(ctx, ref, pullNumber)
	if err != nil {
		return err
	}
	files, err := client.ListPullRequestFiles(ctx, ref, pullNumber, ghadapter.PageOptions{PerPage: 100})
	if err != nil {
		return err
	}
	reviews, err := client.ListPullRequestReviews(ctx, ref, pullNumber, ghadapter.PageOptions{PerPage: 100})
	if err != nil {
		return err
	}
	headSHA := pullRequest.GetHead().GetSHA()
	checks, err := client.ListCheckRunsForRef(ctx, ref, headSHA, ghadapter.CheckRunQuery{Page: ghadapter.PageOptions{PerPage: 100}})
	if err != nil {
		return err
	}
	runs, err := client.ListWorkflowRuns(ctx, ref, ghadapter.WorkflowRunQuery{
		HeadSHA: headSHA,
		Page:    ghadapter.PageOptions{PerPage: 100},
	})
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"pull_request": map[string]any{
			"number": pullRequest.GetNumber(),
			"title":  pullRequest.GetTitle(),
			"state":  pullRequest.GetState(),
			"draft":  pullRequest.GetDraft(),
			"head":   headSHA,
			"base":   pullRequest.GetBase().GetRef(),
			"url":    pullRequest.GetHTMLURL(),
		},
		"changed_files": len(files),
		"reviews":       len(reviews),
		"check_runs":    checks.GetTotal(),
		"workflow_runs": runs.GetTotalCount(),
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
