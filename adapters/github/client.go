package github

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	gh "github.com/google/go-github/v93/github"
	"golang.org/x/oauth2"
)

const defaultTimeout = 30 * time.Second

// Config configures a GitHub REST client. Use TokenSource for renewable
// credentials such as GitHub App installation tokens.
type Config struct {
	Token       string
	TokenSource oauth2.TokenSource
	HTTPClient  *http.Client
	APIURL      string
	UploadURL   string
	Timeout     time.Duration
}

// Client provides GitHub repository, issue, pull request, check, and Actions operations.
type Client struct {
	api *gh.Client
}

// RepositoryRef identifies a repository by its owner and name.
type RepositoryRef struct {
	Owner string
	Name  string
}

// PageOptions selects one page of a GitHub list response.
type PageOptions struct {
	Page    int
	PerPage int
}

// IssueQuery selects issues from a repository. Pull requests are excluded from
// the returned issue list.
type IssueQuery struct {
	State    string
	Assignee string
	Creator  string
	Labels   []string
	Page     PageOptions
}

// PullRequestQuery selects pull requests from a repository.
type PullRequestQuery struct {
	State     string
	Head      string
	Base      string
	Sort      string
	Direction string
	Page      PageOptions
}

// NewPullRequest describes a pull request to create.
type NewPullRequest struct {
	Title               string
	Body                string
	Head                string
	HeadRepository      string
	Base                string
	Draft               bool
	MaintainerCanModify bool
}

// PullRequestUpdate contains fields to change on an existing pull request.
type PullRequestUpdate struct {
	Title               *string
	Body                *string
	State               *string
	MaintainerCanModify *bool
}

// PullRequestReviewInput describes a submitted review and optional inline comments.
type PullRequestReviewInput struct {
	CommitSHA string
	Body      string
	Event     string
	Comments  []PullRequestReviewCommentInput
}

// PullRequestReviewCommentInput identifies a line or range in the pull request diff.
type PullRequestReviewCommentInput struct {
	Path      string
	Body      string
	Line      int
	Side      string
	StartLine int
	StartSide string
	Position  int
}

// PullRequestReviewers identifies users and teams to request or remove.
type PullRequestReviewers struct {
	Users []string
	Teams []string
}

// PendingReviewSubmission describes how to submit a pending review.
type PendingReviewSubmission struct {
	ReviewID int64
	Event    string
	Body     string
}

// MergeOptions specifies an explicit pull request merge request.
type MergeOptions struct {
	Method        string
	CommitTitle   string
	CommitMessage string
	ExpectedHead  string
}

// CheckRunQuery filters check runs attached to a Git reference.
type CheckRunQuery struct {
	CheckName string
	Status    string
	Filter    string
	Page      PageOptions
}

// WorkflowRunQuery filters GitHub Actions workflow runs.
type WorkflowRunQuery struct {
	Actor               string
	Branch              string
	Event               string
	Status              string
	Created             string
	HeadSHA             string
	ExcludePullRequests bool
	CheckSuiteID        int64
	Page                PageOptions
}

// WorkflowDispatch starts a workflow_dispatch run on a branch or tag.
type WorkflowDispatch struct {
	Ref    string
	Inputs map[string]any
}

// NewClient creates a GitHub client. Token and TokenSource are mutually
// exclusive; HTTPClient customizes transport and can be combined with either.
func NewClient(config Config) (*Client, error) {
	if err := validateClientConfig(config); err != nil {
		return nil, err
	}
	options, err := clientOptions(config)
	if err != nil {
		return nil, err
	}
	api, err := gh.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create GitHub REST client: %w", err)
	}
	return &Client{api: api}, nil
}

func validateClientConfig(config Config) error {
	if strings.TrimSpace(config.Token) != "" && config.TokenSource != nil {
		return errors.New("configure either a GitHub token or token source, not both")
	}
	if config.Timeout < 0 {
		return errors.New("GitHub client timeout must not be negative")
	}
	if config.APIURL == "" && config.UploadURL != "" {
		return errors.New("GitHub upload URL requires an API URL")
	}
	return nil
}

func clientOptions(config Config) ([]gh.ClientOptionsFunc, error) {
	options := make([]gh.ClientOptionsFunc, 0, 4)
	if token := strings.TrimSpace(config.Token); token != "" {
		options = append(options, gh.WithAuthToken(token))
	}
	if httpClient := configuredHTTPClient(config); httpClient != nil {
		options = append(options, gh.WithHTTPClient(httpClient))
	} else {
		options = append(options, gh.WithTimeout(clientTimeout(config)))
	}
	if config.APIURL == "" {
		return options, nil
	}
	apiURL, err := normalizeBaseURL(config.APIURL)
	if err != nil {
		return nil, fmt.Errorf("configure GitHub API URL: %w", err)
	}
	var uploadURL *string
	if config.UploadURL != "" {
		value, err := normalizeBaseURL(config.UploadURL)
		if err != nil {
			return nil, fmt.Errorf("configure GitHub upload URL: %w", err)
		}
		uploadURL = &value
	}
	return append(options, gh.WithURLs(&apiURL, uploadURL)), nil
}

func configuredHTTPClient(config Config) *http.Client {
	if config.HTTPClient == nil && config.TokenSource == nil {
		return nil
	}
	client := &http.Client{Timeout: clientTimeout(config)}
	if config.HTTPClient != nil {
		copy := *config.HTTPClient
		client = &copy
		if client.Timeout == 0 || config.Timeout > 0 {
			client.Timeout = clientTimeout(config)
		}
	}
	if config.TokenSource != nil {
		base := client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		client.Transport = &oauth2.Transport{Source: config.TokenSource, Base: base}
	}
	return client
}

func clientTimeout(config Config) time.Duration {
	if config.Timeout > 0 {
		return config.Timeout
	}
	return defaultTimeout
}

func normalizeBaseURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Opaque != "" {
		return "", errors.New("URL must be absolute HTTP(S) without a query, user info, fragment, or opaque component")
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), nil
}

func (ref RepositoryRef) validate() error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Name) == "" ||
		strings.TrimSpace(ref.Owner) != ref.Owner || strings.TrimSpace(ref.Name) != ref.Name ||
		strings.ContainsAny(ref.Owner+ref.Name, "/\\?#%\r\n") || ref.Owner == "." || ref.Owner == ".." || ref.Name == "." || ref.Name == ".." {
		return errors.New("GitHub repository owner and name are required path segments")
	}
	return nil
}

func (page PageOptions) listOptions() *gh.ListOptions {
	pageNumber := page.Page
	if pageNumber < 1 {
		pageNumber = 1
	}
	perPage := page.PerPage
	if perPage < 1 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}
	return &gh.ListOptions{Page: pageNumber, PerPage: perPage}
}

func validateResource(ref RepositoryRef, number int) error {
	if err := ref.validate(); err != nil {
		return err
	}
	if number < 1 {
		return errors.New("GitHub resource number must be positive")
	}
	return nil
}
