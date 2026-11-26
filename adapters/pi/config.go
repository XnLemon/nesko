package pi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultHeartbeatInterval = 20 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
	defaultMaxRecordBytes    = 4 << 20
	defaultStderrBytes       = 32 << 10
)

var defaultTools = []string{"read", "grep", "find", "ls", "bash", "edit", "write"}

// Config selects a Pi CLI process and its workspace boundary.
type Config struct {
	BinaryPath        string
	WorkingDirectory  string // Optional fallback when ExecutionInput has no workspace.
	Provider          string
	Model             string
	Tools             []string
	EnableMCP         bool
	EnableExtensions  bool
	Environment       map[string]string
	HeartbeatInterval time.Duration
	ShutdownTimeout   time.Duration
	MaxRecordBytes    int
}

// Executor runs coding tasks through Pi's language-independent RPC mode.
type Executor struct {
	binaryPath        string
	workingDirectory  string
	args              []string
	environment       map[string]string
	heartbeatInterval time.Duration
	shutdownTimeout   time.Duration
	maxRecordBytes    int
}

// NewExecutor validates the Pi command and workspace before creating an executor.
func NewExecutor(config Config) (*Executor, error) {
	workingDirectory := ""
	var err error
	if strings.TrimSpace(config.WorkingDirectory) != "" {
		workingDirectory, err = resolveWorkingDirectory(config.WorkingDirectory)
		if err != nil {
			return nil, err
		}
	}
	tools, err := normalizeTools(config.Tools)
	if err != nil {
		return nil, err
	}
	environment, err := copyEnvironment(config.Environment)
	if err != nil {
		return nil, err
	}

	provider := strings.TrimSpace(config.Provider)
	model := strings.TrimSpace(config.Model)
	if provider != "" && model == "" {
		return nil, errors.New("Pi provider requires a model")
	}
	binaryPath := strings.TrimSpace(config.BinaryPath)
	if binaryPath == "" {
		binaryPath = "pi"
	}

	return &Executor{
		binaryPath:        binaryPath,
		workingDirectory:  workingDirectory,
		args:              buildArguments(tools, provider, model, config.EnableMCP, config.EnableExtensions),
		environment:       environment,
		heartbeatInterval: positiveDuration(config.HeartbeatInterval, defaultHeartbeatInterval),
		shutdownTimeout:   positiveDuration(config.ShutdownTimeout, defaultShutdownTimeout),
		maxRecordBytes:    positiveInt(config.MaxRecordBytes, defaultMaxRecordBytes),
	}, nil
}

func resolveWorkingDirectory(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", errors.New("Pi working directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve Pi working directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect Pi working directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("Pi working directory must be a directory")
	}
	return absolute, nil
}

func normalizeTools(tools []string) ([]string, error) {
	if len(tools) == 0 {
		tools = defaultTools
	}
	normalized := make([]string, len(tools))
	for index, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool == "" || strings.Contains(tool, ",") {
			return nil, errors.New("Pi tool names must be non-empty and must not contain commas")
		}
		normalized[index] = tool
	}
	return normalized, nil
}

func copyEnvironment(environment map[string]string) (map[string]string, error) {
	copyOfEnvironment := make(map[string]string, len(environment))
	for key, value := range environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("invalid Pi environment entry for key %q", key)
		}
		copyOfEnvironment[key] = value
	}
	return copyOfEnvironment, nil
}

func buildArguments(tools []string, provider, model string, enableMCP, enableExtensions bool) []string {
	args := []string{"--mode", "rpc", "--no-session", "--tools", strings.Join(tools, ",")}
	if !enableMCP {
		args = append(args, "--no-mcp")
	}
	if !enableExtensions {
		args = append(args, "--no-extensions")
	}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
