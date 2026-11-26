package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/xnlemon/nesko"
	"github.com/xnlemon/nesko/adapters/feishu"
	"github.com/xnlemon/nesko/adapters/judge"
	"github.com/xnlemon/nesko/adapters/openai"
	"github.com/xnlemon/nesko/adapters/pi"
	"github.com/xnlemon/nesko/adapters/sqlite"
	"github.com/xnlemon/nesko/ports"
	"github.com/xnlemon/nesko/service"
)

const maxProjectContextBytes = 96 << 10

type sampleConfig struct {
	requestText        string
	feishuAppID        string
	feishuAppSecret    string
	feishuBotOpenID    string
	feishuAllowedIDs   map[string]struct{}
	judgeAuth          string
	judgeEndpoint      string
	judgeModel         string
	judgeAPIKey        string
	oauthCredentials   string
	projectContextFile string
	databasePath       string
	piBinaryPath       string
	piWorkingDirectory string
	piProvider         string
	piModel            string
	piTools            []string
}

type judgeSettings struct {
	auth             string
	endpoint         string
	provider         string
	piModel          string
	oauthCredentials string
}

func main() {
	if err := runWithSignals(); err != nil {
		slog.Error("Nesko core-loop sample failed", "error", err)
		os.Exit(1)
	}
}

func runWithSignals() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx)
}

func run(ctx context.Context) (runErr error) {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	repository, err := sqlite.NewTicketRepository(config.databasePath)
	if err != nil {
		return fmt.Errorf("open ticket repository: %w", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close ticket repository: %w", err))
		}
	}()

	workflow, err := newWorkflow(config, repository)
	if err != nil {
		return err
	}
	if config.feishuAppID != "" {
		return runFeishuGateway(ctx, config, repository, workflow)
	}
	ticket, err := createTicket(ctx, repository, config.requestText)
	if err != nil {
		return err
	}
	result, err := workflow.Triage(ctx, ticket.ID)
	if err != nil {
		return fmt.Errorf("triage sample ticket: %w", err)
	}
	return printTicketResult(result)
}

func loadConfig() (sampleConfig, error) {
	feishuAppID := strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
	feishuAppSecret := strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	feishuBotOpenID := strings.TrimSpace(os.Getenv("FEISHU_BOT_OPEN_ID"))
	var requestText string
	var err error
	if feishuAppID == "" {
		if feishuAppSecret != "" || feishuBotOpenID != "" {
			return sampleConfig{}, errors.New("FEISHU_APP_ID is required when other Feishu settings are set")
		}
		requestText, err = requiredEnvironment("NESKO_REQUEST_TEXT")
		if err != nil {
			return sampleConfig{}, err
		}
	} else if feishuAppSecret == "" || feishuBotOpenID == "" {
		return sampleConfig{}, errors.New("FEISHU_APP_SECRET and FEISHU_BOT_OPEN_ID are required in Feishu mode")
	}
	judgeModel, err := requiredEnvironment("NESKO_JUDGE_MODEL")
	if err != nil {
		return sampleConfig{}, err
	}
	settings, err := loadJudgeSettings(judgeModel)
	if err != nil {
		return sampleConfig{}, err
	}
	workingDirectory, err := requiredEnvironment("PI_WORKING_DIRECTORY")
	if err != nil {
		return sampleConfig{}, err
	}
	databasePath := strings.TrimSpace(os.Getenv("NESKO_DATABASE_PATH"))
	if databasePath == "" {
		databasePath = "./data/nesko-sample.sqlite"
	}
	return sampleConfig{
		requestText:        requestText,
		feishuAppID:        feishuAppID,
		feishuAppSecret:    feishuAppSecret,
		feishuBotOpenID:    feishuBotOpenID,
		feishuAllowedIDs:   stringSet(os.Getenv("FEISHU_ALLOWED_OPEN_IDS")),
		judgeAuth:          settings.auth,
		judgeEndpoint:      settings.endpoint,
		judgeModel:         judgeModel,
		judgeAPIKey:        os.Getenv("NESKO_JUDGE_API_KEY"),
		oauthCredentials:   settings.oauthCredentials,
		projectContextFile: strings.TrimSpace(os.Getenv("NESKO_PROJECT_CONTEXT_FILE")),
		databasePath:       databasePath,
		piBinaryPath:       os.Getenv("PI_BINARY_PATH"),
		piWorkingDirectory: workingDirectory,
		piProvider:         settings.provider,
		piModel:            settings.piModel,
		piTools:            splitList(os.Getenv("PI_TOOLS")),
	}, nil
}

func runFeishuGateway(ctx context.Context, config sampleConfig, repository *sqlite.TicketRepository, workflow ports.TicketWorkflow) error {
	messenger, err := feishu.NewMessenger(config.feishuAppID, config.feishuAppSecret)
	if err != nil {
		return fmt.Errorf("configure Feishu messenger: %w", err)
	}
	intake, err := service.NewReceiveRequest(repository, messenger, slog.Default())
	if err != nil {
		return fmt.Errorf("configure request intake: %w", err)
	}
	handler := &intakeWorkflowHandler{
		intake:   intake,
		tickets:  repository,
		workflow: workflow,
		logger:   slog.Default(),
	}
	gateway, err := feishu.NewGateway(feishu.GatewayConfig{
		AppID:          config.feishuAppID,
		AppSecret:      config.feishuAppSecret,
		BotOpenID:      config.feishuBotOpenID,
		AllowedOpenIDs: config.feishuAllowedIDs,
	}, handler, slog.Default())
	if err != nil {
		return fmt.Errorf("configure Feishu gateway: %w", err)
	}
	defer gateway.Close()
	if err := gateway.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run Feishu gateway: %w", err)
	}
	return nil
}

type intakeWorkflowHandler struct {
	intake   *service.ReceiveRequest
	tickets  ports.TicketRepository
	workflow ports.TicketWorkflow
	logger   *slog.Logger
}

func (h *intakeWorkflowHandler) Handle(ctx context.Context, request nesko.IncomingRequest) error {
	ticket, err := h.intake.Receive(ctx, request)
	if err != nil {
		return err
	}
	if ticket.Status != nesko.TicketStatusReceived {
		return nil
	}
	ticket, err = h.workflow.Assess(ctx, ticket.ID)
	if errors.Is(err, nesko.ErrTicketStatusConflict) {
		current, getErr := h.tickets.Get(ctx, ticket.ID)
		if getErr == nil && current.Status != nesko.TicketStatusReceived {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("assess Feishu ticket %s: %w", ticket.ID, err)
	}
	if ticket.Status == nesko.TicketStatusQueued {
		ticket, err = h.workflow.Execute(ctx, ticket.ID)
		if err != nil {
			return fmt.Errorf("execute Feishu ticket %s: %w", ticket.ID, err)
		}
	}
	h.logger.Info("Feishu ticket processed", "ticket_id", ticket.ID, "status", ticket.Status)
	return nil
}

func loadJudgeSettings(judgeModel string) (judgeSettings, error) {
	settings := judgeSettings{
		auth:     strings.ToLower(strings.TrimSpace(os.Getenv("NESKO_JUDGE_AUTH"))),
		provider: strings.TrimSpace(os.Getenv("PI_PROVIDER")),
		piModel:  strings.TrimSpace(os.Getenv("PI_MODEL")),
	}
	if settings.auth == "" {
		settings.auth = "api-key"
	}
	if settings.auth != "api-key" && settings.auth != "chatgpt-oauth" {
		return judgeSettings{}, errors.New("NESKO_JUDGE_AUTH must be api-key or chatgpt-oauth")
	}
	if settings.auth == "api-key" {
		endpoint, err := requiredEnvironment("NESKO_JUDGE_ENDPOINT")
		if err != nil {
			return judgeSettings{}, err
		}
		settings.endpoint = endpoint
		return settings, nil
	}
	credentialsPath := strings.TrimSpace(os.Getenv("NESKO_OPENAI_OAUTH_CREDENTIALS_PATH"))
	if credentialsPath == "" {
		defaultPath, err := openai.DefaultCredentialsPath()
		if err != nil {
			return judgeSettings{}, fmt.Errorf("resolve default OpenAI OAuth credentials path: %w", err)
		}
		credentialsPath = defaultPath
	}
	settings.oauthCredentials = credentialsPath
	return configureChatGPTOAuth(settings, judgeModel)
}

func configureChatGPTOAuth(settings judgeSettings, judgeModel string) (judgeSettings, error) {
	if settings.provider == "" {
		settings.provider = "openai-codex"
	}
	if settings.provider != "openai-codex" {
		return judgeSettings{}, errors.New("PI_PROVIDER must be openai-codex when NESKO_JUDGE_AUTH is chatgpt-oauth")
	}
	if settings.piModel == "" {
		settings.piModel = judgeModel
	}
	return settings, nil
}

func newWorkflow(config sampleConfig, repository *sqlite.TicketRepository) (*service.TicketWorkflow, error) {
	modelClient, err := newJudgeClient(config)
	if err != nil {
		return nil, err
	}
	contextProvider, err := projectContextProvider(config.projectContextFile)
	if err != nil {
		return nil, err
	}
	modelJudge, err := judge.NewModelJudge(judge.ModelJudgeConfig{
		Client:          modelClient,
		ContextProvider: contextProvider,
	})
	if err != nil {
		return nil, fmt.Errorf("configure model Judge: %w", err)
	}
	piExecutor, err := pi.NewExecutor(pi.Config{
		BinaryPath:       config.piBinaryPath,
		WorkingDirectory: config.piWorkingDirectory,
		Provider:         config.piProvider,
		Model:            config.piModel,
		Tools:            config.piTools,
	})
	if err != nil {
		return nil, fmt.Errorf("configure Pi executor: %w", err)
	}
	workflow, err := service.NewTicketWorkflow(service.TicketWorkflowDependencies{
		Tickets:   repository,
		Judge:     modelJudge,
		Runs:      repository,
		Lifecycle: repository,
		Router: ports.ExecutorRouterFunc(func(context.Context, nesko.ExecutionInput) (ports.Executor, error) {
			return piExecutor, nil
		}),
		Logger: slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("configure ticket workflow: %w", err)
	}
	return workflow, nil
}

func newJudgeClient(config sampleConfig) (judge.CompletionClient, error) {
	switch config.judgeAuth {
	case "chatgpt-oauth":
		return newChatGPTOAuthJudgeClient(config)
	case "api-key":
		return newAPIKeyJudgeClient(config)
	default:
		return nil, fmt.Errorf("unsupported Judge auth mode %q", config.judgeAuth)
	}
}

func newChatGPTOAuthJudgeClient(config sampleConfig) (judge.CompletionClient, error) {
	tokenSource, err := openai.NewOAuthTokenSource(openai.OAuthTokenSourceConfig{
		CredentialsPath: config.oauthCredentials,
	})
	if err != nil {
		return nil, fmt.Errorf("configure OpenAI ChatGPT OAuth token source: %w", err)
	}
	client, err := judge.NewOpenAIResponsesClient(judge.OpenAIResponsesConfig{
		Model:             config.judgeModel,
		AccessTokenSource: tokenSource,
	})
	if err != nil {
		return nil, fmt.Errorf("configure Judge Responses client: %w", err)
	}
	return client, nil
}

func newAPIKeyJudgeClient(config sampleConfig) (judge.CompletionClient, error) {
	client, err := judge.NewOpenAICompatibleClient(judge.OpenAICompatibleConfig{
		Endpoint: config.judgeEndpoint,
		Model:    config.judgeModel,
		APIKey:   config.judgeAPIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("configure Judge client: %w", err)
	}
	return client, nil
}

func createTicket(ctx context.Context, repository *sqlite.TicketRepository, requestText string) (nesko.Ticket, error) {
	ticket, err := repository.CreateOrGet(ctx, nesko.IncomingRequest{
		Channel:        "sample",
		MessageID:      uuid.NewString(),
		ConversationID: "sample",
		RequesterID:    "sample",
		Text:           requestText,
		ReceivedAt:     time.Now().UTC(),
	})
	if err != nil {
		return nesko.Ticket{}, fmt.Errorf("create sample ticket: %w", err)
	}
	return ticket, nil
}

func printTicketResult(ticket nesko.Ticket) error {
	output, err := json.MarshalIndent(struct {
		TicketID string                `json:"ticket_id"`
		Number   int64                 `json:"number"`
		Status   nesko.TicketStatus    `json:"status"`
		Decision *nesko.TicketDecision `json:"decision,omitempty"`
		Summary  string                `json:"execution_summary,omitempty"`
	}{
		TicketID: ticket.ID,
		Number:   ticket.Number,
		Status:   ticket.Status,
		Decision: ticket.Decision,
		Summary:  ticket.ExecutionSummary,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sample result: %w", err)
	}
	fmt.Println(string(output))
	return nil
}

func requiredEnvironment(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func projectContextProvider(path string) (provider judge.AssessmentContextProvider, readErr error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open project context file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			readErr = errors.Join(readErr, fmt.Errorf("close project context file: %w", err))
		}
	}()
	content, err := io.ReadAll(io.LimitReader(file, maxProjectContextBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read project context file: %w", err)
	}
	if len(content) > maxProjectContextBytes {
		return nil, fmt.Errorf("project context file exceeds %d bytes", maxProjectContextBytes)
	}
	return judge.AssessmentContextFunc(func(context.Context, nesko.Ticket) (string, error) {
		return string(content), nil
	}), nil
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	items := strings.Split(value, ",")
	for index := range items {
		items[index] = strings.TrimSpace(items[index])
	}
	return items
}

func stringSet(value string) map[string]struct{} {
	items := splitList(value)
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}
