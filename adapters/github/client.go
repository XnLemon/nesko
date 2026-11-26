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

// Client provides GitHub repository, issue, and pull request operations.
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

// MergeOptions specifies an explicit pull request merge request.
type MergeOptions struct {
	Method        string
	CommitTitle   string
	CommitMessage string
	ExpectedHead  string
}

// NewClient creates a GitHub client. Token and TokenSource are mutually
// exclusive; HTTPClient customizes transport and can be combined with either.
func NewClient(config Config) (*Client, error) {
	if strings.TrimSpace(config.Token) != "" && config.TokenSource != nil {
		return nil, errors.New("configure either a GitHub token or token source, not both")
	}
	if config.Timeout < 0 {
		return nil, errors.New("GitHub client timeout must not be negative")
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	options := make([]gh.ClientOptionsFunc, 0, 4)
	if strings.TrimSpace(config.Token) != "" {
		options = append(options, gh.WithAuthToken(strings.TrimSpace(config.Token)))
	}
	if config.HTTPClient != nil || config.TokenSource != nil {
		httpClient := &http.Client{Timeout: timeout}
		if config.HTTPClient != nil {
			copy := *config.HTTPClient
			httpClient = &copy
			if httpClient.Timeout == 0 || config.Timeout > 0 {
				httpClient.Timeout = timeout
			}
		}
		if config.TokenSource != nil {
			transport := httpClient.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			httpClient.Transport = &oauth2.Transport{Source: config.TokenSource, Base: transport}
		}
		options = append(options, gh.WithHTTPClient(httpClient))
	} else {
		options = append(options, gh.WithTimeout(timeout))
	}
	if config.APIURL != "" {
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
		options = append(options, gh.WithURLs(&apiURL, uploadURL))
	} else if config.UploadURL != "" {
		return nil, errors.New("GitHub upload URL requires an API URL")
	}

	api, err := gh.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create GitHub REST client: %w", err)
	}
	return &Client{api: api}, nil
}

func normalizeBaseURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("URL must be an absolute HTTP(S) URL without user info or fragment")
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), nil
}

func (ref RepositoryRef) validate() error {
	if strings.TrimSpace(ref.Owner) == "" || strings.TrimSpace(ref.Name) == "" ||
		strings.TrimSpace(ref.Owner) != ref.Owner || strings.TrimSpace(ref.Name) != ref.Name ||
		strings.ContainsAny(ref.Owner+ref.Name, "/\\\r\n") {
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
