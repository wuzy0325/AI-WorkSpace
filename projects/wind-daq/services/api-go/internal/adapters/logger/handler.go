package logger

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalSink LogSink
	sinkMu     sync.RWMutex
)

// LogSink receives structured log entries for frontend push.
type LogSink interface {
	Send(entry any)
}

// LogEntry is a structured log record sent to the frontend.
type LogEntry struct {
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Timestamp string                 `json:"timestamp"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// SetSink sets the global log sink (called once during init).
func SetSink(s LogSink) {
	sinkMu.Lock()
	globalSink = s
	sinkMu.Unlock()
}

// Init initializes the global slog logger with the given config.
// If sink is non-nil, log entries above cfg.FrontendMinLevel are forwarded.
func Init(cfg Config, sink LogSink) {
	if err := cfg.Validate(); err != nil {
		slog.Warn("Logging config validation failed, using defaults", "err", err)
		cfg = DefaultConfig()
	}

	SetSink(sink)

	var handlers []slog.Handler
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}

	if cfg.Console {
		if cfg.Format == "json" {
			handlers = append(handlers, slog.NewJSONHandler(os.Stderr, opts))
		} else {
			handlers = append(handlers, slog.NewTextHandler(os.Stderr, opts))
		}
	}

	if cfg.File.Enabled {
		w := &lumberjack.Logger{
			Filename:   cfg.File.Path,
			MaxSize:    cfg.File.MaxSizeMB,
			MaxBackups: cfg.File.MaxBackups,
			MaxAge:     cfg.File.MaxAgeDays,
			Compress:   cfg.File.Compress,
		}
		handlers = append(handlers, slog.NewJSONHandler(w, opts))
	}

	if cfg.Frontend.Enabled && sink != nil {
		handlers = append(handlers, newSinkHandler(cfg.FrontendMinLevel()))
	}

	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, opts))
	}

	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = &multiHandler{handlers: handlers}
	}

	slog.SetDefault(slog.New(handler))
}

// --- multiHandler: fans out to multiple slog.Handlers ---

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}

// --- sinkHandler: forwards slog records to LogSink ---

type sinkHandler struct {
	level slog.Level
}

func newSinkHandler(level slog.Level) *sinkHandler {
	return &sinkHandler{level: level}
}

func (s *sinkHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level
}

func (s *sinkHandler) Handle(_ context.Context, r slog.Record) error {
	sinkMu.RLock()
	sk := globalSink
	sinkMu.RUnlock()
	if sk == nil {
		return nil
	}

	entry := LogEntry{
		Level:     r.Level.String(),
		Message:   r.Message,
		Timestamp: r.Time.Format("2006-01-02T15:04:05.000Z07:00"),
	}

	r.Attrs(func(a slog.Attr) bool {
		if entry.Fields == nil {
			entry.Fields = make(map[string]interface{})
		}
		entry.Fields[a.Key] = a.Value.Any()
		return true
	})

	sk.Send(entry)
	return nil
}

func (s *sinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return s }
func (s *sinkHandler) WithGroup(name string) slog.Handler        { return s }

// --- compile-time interface checks ---

var _ slog.Handler = (*multiHandler)(nil)
var _ slog.Handler = (*sinkHandler)(nil)

// DiscardSink is a no-op sink for testing.
type DiscardSink struct{}

func (DiscardSink) Send(any) {}
