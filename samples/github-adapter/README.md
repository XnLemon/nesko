# GitHub Adapter Sample

This read-only sample loads a repository and, optionally, a pull request with its changed-file count, reviews, check runs, and Actions runs. It uses the credentials available through `GH_TOKEN` or `GITHUB_TOKEN`; public repositories can be queried without a token, subject to GitHub's unauthenticated rate limit.

```sh
go run ./samples/github-adapter owner/repository
go run ./samples/github-adapter owner/repository 42
```

Mutating operations such as fork creation, review submission, workflow dispatch, and merge are exposed on `adapters/github.Client`; the sample does not invoke them.
