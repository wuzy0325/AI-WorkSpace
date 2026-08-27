package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"cal1604/internal/events"
)

type sseEventPayload struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

func publishEvent(eventType string, data any) {
	events.GlobalBus.Publish(events.Event{Type: eventType, Data: data})
}

func eventsStreamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsubscribe := events.GlobalBus.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}

			payload, err := json.Marshal(sseEventPayload{Type: evt.Type, Data: evt.Data})
			if err != nil {
				continue
			}

			if _, err = fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
