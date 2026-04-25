package gospike

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hanthor/oramalama/internal/api"
	"github.com/hanthor/oramalama/internal/readline"
	"github.com/hanthor/oramalama/internal/tui"
)

type interactive struct {
	app      *App
	client   *api.Client
	endpoint string
	model    string
	reader   *readline.Instance
	ctxSize  int
	history  []string
	system   string
	running  bool
}

func (a *App) interactive(ctx context.Context, model string) error {
	models, err := a.installedModels(ctx)
	if err != nil {
		return err
	}

	model, err = selectModel(ctx, model, models, a.stdout, a.stderr)
	if err != nil {
		return err
	}

	if model == "" {
		return errors.New("no model selected")
	}

	endpoint, servedModel, err := a.ensureServer(ctx, model, false)
	if err != nil {
		return err
	}

	client, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	ctxSize := getCtxSize(servedModel)

	i := &interactive{
		app:      a,
		client:   client,
		endpoint: endpoint,
		model:    servedModel,
		ctxSize:  ctxSize,
	}

	return i.run(ctx)
}

func selectModel(ctx context.Context, model string, models []modelInfo, stdout, stderr io.Writer) (string, error) {
	if model != "" {
		return model, nil
	}

	if len(models) == 0 {
		fmt.Fprintln(stderr, "no models installed. Run: oramalama-go pull <model>")
		return "", nil
	}

	items := make([]tui.SelectItem, 0, len(models))
	for _, m := range models {
		desc := ""
		if m.Size > 0 {
			desc = fmt.Sprintf("%.1f GB", float64(m.Size)/1024.0/1024.0/1024.0)
		}
		items = append(items, tui.SelectItem{
			Name:        m.Name,
			Description: desc,
			Recommended: false,
		})
	}

	selected, err := tui.SelectSingle("Select a model", items, "")
	if err != nil {
		if errors.Is(err, tui.ErrCancelled) {
			return "", nil
		}
		return "", fmt.Errorf("model selection: %w", err)
	}

	return selected, nil
}

func (i *interactive) run(ctx context.Context) error {
	reader, err := readline.New(readline.Prompt{
		Prompt:      "> ",
		AltPrompt:   "... ",
		Placeholder: "Send a message (tab for tools, / for commands, ctrl+D to exit)",
	})
	if err != nil {
		return fmt.Errorf("failed to create readline: %w", err)
	}
	i.reader = reader

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		i.cancel()
	}()

	fmt.Fprintf(i.app.stdout, "Connected to %s\nModel: %s\nContext: %d tokens\nType /help for commands\n\n", i.endpoint, i.model, i.ctxSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := i.readLine()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) {
				fmt.Fprintln(i.app.stdout)
				continue
			}
			if errors.Is(err, readline.ErrEditPrompt) {
				continue
			}
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(i.app.stdout)
				break
			}
			return fmt.Errorf("read error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if err := i.handleCommand(line); err != nil {
				fmt.Fprintf(i.app.stderr, "%s\n", err)
			}
			continue
		}

		if err := i.handleMessage(ctx, line); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(i.app.stdout, "\n[interrupted]")
				continue
			}
			return fmt.Errorf("message error: %w", err)
		}
	}

	return nil
}

func (i *interactive) readLine() (string, error) {
	return i.reader.Readline()
}

func (i *interactive) cancel() {
	// Signal handling is done via context
}

func (i *interactive) handleCommand(line string) error {
	args := strings.Fields(line)
	cmd := args[0]

	switch cmd {
	case "/help":
		fmt.Fprintln(i.app.stdout, `Commands:
  /help          Show this help
  /model <name>  Switch model
  /system <text> Set system prompt
  /clear         Clear conversation history
  /history       Show conversation history
  /ctx <size>    Show current context size
  /exit          Exit interactive mode`)
		return nil

	case "/exit", "/quit":
		return errors.New("/exit")

	case "/model":
		if len(args) < 2 {
			return errors.New("usage: /model <name>")
		}
		newModel := args[1]
		return i.switchModel(newModel)

	case "/system":
		if len(args) < 2 {
			return errors.New("usage: /system <text>")
		}
		i.system = strings.Join(args[1:], " ")
		fmt.Fprintf(i.app.stdout, "System prompt set\n")
		return nil

	case "/clear":
		i.history = nil
		fmt.Fprintf(i.app.stdout, "Conversation cleared\n")
		return nil

	case "/history":
		for _, msg := range i.history {
			fmt.Fprintf(i.app.stdout, "%s\n", msg)
		}
		return nil

	case "/ctx":
		fmt.Fprintf(i.app.stdout, "Context size: %d tokens\n", i.ctxSize)
		return nil

	default:
		return fmt.Errorf("unknown command: %s (type /help for commands)", cmd)
	}
}

func (i *interactive) switchModel(newModel string) error {
	// Check if model is installed
	models, err := i.app.installedModels(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	found := false
	for _, m := range models {
		if m.Name == newModel {
			found = true
			break
		}
	}

	if !found {
		// Ask user if they want to pull the model
		confirmed, err := tui.RunConfirm(fmt.Sprintf("Model %q not found. Pull it now?", newModel))
		if err != nil {
			return fmt.Errorf("confirmation: %w", err)
		}
		if !confirmed {
			return nil
		}

		pullProgress := func(resp api.ProgressResponse) error {
			fmt.Fprintf(i.app.stdout, "%s: %d / %d\n", resp.Status, resp.Completed, resp.Total)
			return nil
		}

		pullReq := &api.PullRequest{Model: newModel}
		if err := i.client.Pull(context.Background(), pullReq, pullProgress); err != nil {
			return fmt.Errorf("failed to pull model: %w", err)
		}
	}

	i.model = newModel
	i.history = nil
	fmt.Fprintf(i.app.stdout, "Switched to model: %s\n", newModel)
	return nil
}

func (i *interactive) handleMessage(ctx context.Context, message string) error {
	i.history = append(i.history, message)

	req := &api.ChatRequest{
		Model:    i.model,
		Messages: buildMessages(i.history, i.system),
		Stream:   boolPtr(true),
	}

	var fullResponse strings.Builder
	fmt.Fprint(i.app.stdout, "> ")

	err := i.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		content := messageContentString(resp.Message.Content)
		if content != "" {
			fmt.Fprint(i.app.stdout, content)
			fullResponse.WriteString(content)
		}
		return nil
	})

	fmt.Fprintln(i.app.stdout)

	if err != nil {
		return err
	}

	if fullResponse.Len() > 0 {
		i.history = append(i.history, fullResponse.String())
	}

	return nil
}

func buildMessages(history []string, system string) []api.Message {
	messages := make([]api.Message, 0)

	if system != "" {
		messages = append(messages, api.Message{
			Role:    "system",
			Content: system,
		})
	}

	for j := 0; j < len(history); j += 2 {
		userMsg := history[j]
		messages = append(messages, api.Message{
			Role:    "user",
			Content: userMsg,
		})

		if j+1 < len(history) {
			assistantMsg := history[j+1]
			messages = append(messages, api.Message{
				Role:    "assistant",
				Content: assistantMsg,
			})
		}
	}

	return messages
}

func boolPtr(b bool) *bool {
	return &b
}

func messageContentString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
