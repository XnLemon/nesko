package pi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xnlemon/nesko"
)

type rpcLine struct {
	data []byte
	err  error
}

type rpcCommand struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

type rpcRecord struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Success  bool            `json:"success"`
	Error    string          `json:"error"`
	Data     json.RawMessage `json:"data"`
	Aborted  bool            `json:"aborted"`
	ToolName string          `json:"toolName"`
	IsError  bool            `json:"isError"`
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(data) >= b.limit {
		b.data = append(b.data[:0], data[len(data)-b.limit:]...)
		return len(data), nil
	}
	if overflow := len(b.data) + len(data) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}

func readRPC(reader io.Reader, output chan<- rpcLine, stopped <-chan struct{}, maxRecordBytes int) {
	defer close(output)
	scanner := bufio.NewScanner(reader)
	initialBuffer := min(64<<10, maxRecordBytes)
	if initialBuffer < 1 {
		initialBuffer = 1
	}
	scanner.Buffer(make([]byte, initialBuffer), maxRecordBytes)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		select {
		case output <- rpcLine{data: line}:
		case <-stopped:
			return
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case output <- rpcLine{err: err}:
		case <-stopped:
		}
	}
}

func sendCommand(writer io.Writer, command rpcCommand) error {
	data, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("encode Pi RPC command: %w", err)
	}
	data = append(data, '\n')
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func decodeRPCRecord(data []byte) (rpcRecord, error) {
	var record rpcRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return rpcRecord{}, fmt.Errorf("decode Pi RPC record: %w", err)
	}
	if record.Type == "" {
		return rpcRecord{}, errors.New("decode Pi RPC record: missing type")
	}
	return record, nil
}

func responseDisposition(data json.RawMessage) (string, error) {
	var response struct {
		Disposition string `json:"disposition"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decode Pi prompt disposition: %w", err)
	}
	switch response.Disposition {
	case "started", "queued", "handled":
		return response.Disposition, nil
	default:
		return "", fmt.Errorf("pi returned unsupported prompt disposition %q", response.Disposition)
	}
}

func reportProgress(report func(nesko.ExecutionEvent) error, detail string) error {
	if report == nil {
		return nil
	}
	return report(nesko.ExecutionEvent{Type: nesko.ExecutionEventProgress, At: time.Now().UTC(), Detail: detail})
}

func reportHeartbeat(report func(nesko.ExecutionEvent) error) error {
	if report == nil {
		return nil
	}
	return report(nesko.ExecutionEvent{Type: nesko.ExecutionEventHeartbeat, At: time.Now().UTC()})
}

func safeDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return value
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

var _ io.Writer = (*tailBuffer)(nil)
