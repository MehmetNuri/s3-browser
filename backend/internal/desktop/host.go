package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

type Message struct {
	Type   string            `json:"type"`
	ID     int64             `json:"id,omitempty"`
	Method string            `json:"method,omitempty"`
	Args   []json.RawMessage `json:"args,omitempty"`
	Event  string            `json:"event,omitempty"`
	Data   any               `json:"data,omitempty"`
	Result any               `json:"result,omitempty"`
	Error  string            `json:"error,omitempty"`
}
type Host struct {
	writeMu sync.Mutex
	writer  *json.Encoder
	mu      sync.Mutex
	pending map[int64]chan Message
	nextID  atomic.Int64
}
type hostKey struct{}

func NewHost(output io.Writer) *Host {
	return &Host{writer: json.NewEncoder(output), pending: make(map[int64]chan Message)}
}
func WithHost(ctx context.Context, host *Host) context.Context {
	return context.WithValue(ctx, hostKey{}, host)
}
func (h *Host) Send(message Message) error {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	return h.writer.Encode(message)
}
func (h *Host) Read(input io.Reader, handle func(Message)) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var message Message
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			return errors.New("Invalid desktop protocol message")
		}
		switch message.Type {
		case "request":
			handle(message)
		case "host-response":
			h.mu.Lock()
			pending := h.pending[message.ID]
			h.mu.Unlock()
			if pending != nil {
				select {
				case pending <- message:
				default:
				}
			}
		default:
			return errors.New("Unknown desktop protocol message")
		}
	}
	return scanner.Err()
}
func Call(ctx context.Context, method string, args any, result any) error {
	if ctx == nil {
		return errors.New("Desktop host is unavailable")
	}
	h, ok := ctx.Value(hostKey{}).(*Host)
	if !ok {
		return errors.New("Desktop host is unavailable")
	}
	data, err := json.Marshal(args)
	if err != nil {
		return err
	}
	id := h.nextID.Add(1)
	replies := make(chan Message, 1)
	h.mu.Lock()
	h.pending[id] = replies
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.pending, id); h.mu.Unlock() }()
	if err := h.Send(Message{Type: "host-request", ID: id, Method: method, Args: []json.RawMessage{data}}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case reply := <-replies:
		if reply.Error != "" {
			return errors.New(reply.Error)
		}
		if result == nil {
			return nil
		}
		data, err := json.Marshal(reply.Result)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, result)
	}
}
func EventsEmit(ctx context.Context, name string, data any) {
	if ctx == nil {
		return
	}
	if h, ok := ctx.Value(hostKey{}).(*Host); ok {
		_ = h.Send(Message{Type: "event", Event: name, Data: data})
	}
}
