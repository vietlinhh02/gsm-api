package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type orderStreamKey struct {
	environment string
	client      string
	id          string
}

type orderHub struct {
	mu          sync.Mutex
	subscribers map[orderStreamKey]map[chan struct{}]struct{}
}

func (h *orderHub) subscribe(key orderStreamKey) (<-chan struct{}, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers == nil {
		h.subscribers = make(map[orderStreamKey]map[chan struct{}]struct{})
	}
	if h.subscribers[key] == nil {
		h.subscribers[key] = make(map[chan struct{}]struct{})
	}
	changed := make(chan struct{}, 1)
	h.subscribers[key][changed] = struct{}{}
	return changed, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subscribers[key], changed)
		if len(h.subscribers[key]) == 0 {
			delete(h.subscribers, key)
		}
	}
}

func (a *API) publishOrder(environment, owner, id string) {
	a.orderHub.mu.Lock()
	defer a.orderHub.mu.Unlock()
	for changed := range a.orderHub.subscribers[orderStreamKey{environment, owner, id}] {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}

func (a *API) streamOrder(w http.ResponseWriter, r *http.Request) {
	key := orderStreamKey{requestEnvironment(r.Context()), clientID(r.Context()), r.PathValue("id")}
	changed, unsubscribe := a.orderHub.subscribe(key)
	defer unsubscribe()
	payload, err := a.streamOrderPayload(r.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, 4031, "order not found")
		return
	}
	if err != nil {
		a.dbError(w, "load order stream", err)
		return
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		if err := writeOrderFrame(w, controller, payload); err != nil {
			return
		}
		if payload["status"] == "COMPLETED" || payload["status"] == "CANCELLED" {
			return
		}
		if !waitOrderChange(r, w, controller, changed, heartbeat.C) {
			return
		}
		payload, err = a.streamOrderPayload(r.Context(), key)
		if err != nil {
			a.Logger.Printf("refresh order stream: %v", err)
			return
		}
	}
}

func (a *API) streamOrderPayload(
	ctx context.Context, key orderStreamKey,
) (map[string]any, error) {
	var payload map[string]any
	err := a.Store.DB.QueryRowContext(ctx, `
  SELECT payload FROM orders WHERE id=$1 AND environment=$2 AND client_id=$3
 `, key.id, key.environment, key.client).Scan(jsonScanner{target: &payload})
	return payload, err
}

func writeOrderFrame(
	w http.ResponseWriter, controller *http.ResponseController, payload any,
) error {
	encoded, err := json.Marshal(map[string]any{"data": payload})
	if err != nil {
		return err
	}
	return writeStreamFrame(w, controller, "event: order\ndata: "+string(encoded)+"\n\n")
}

func writeStreamFrame(
	w http.ResponseWriter, controller *http.ResponseController, frame string,
) error {
	if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, frame); err != nil {
		return err
	}
	return controller.Flush()
}

func waitOrderChange(r *http.Request, w http.ResponseWriter, controller *http.ResponseController,
	changed <-chan struct{}, heartbeat <-chan time.Time,
) bool {
	for {
		select {
		case <-r.Context().Done():
			return false
		case <-changed:
			return true
		case <-heartbeat:
			if err := writeStreamFrame(w, controller, ": keep-alive\n\n"); err != nil {
				return false
			}
		}
	}
}
