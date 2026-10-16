package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxWebhookBody = 256 << 10

type webhookInbox struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	ReceivePath string    `json:"receive_path"`
	CreatedAt   time.Time `json:"created_at"`
}

type webhookReceipt struct {
	ID         int64             `json:"id"`
	Headers    map[string]string `json:"headers,omitempty"`
	Preview    string            `json:"preview"`
	Size       int               `json:"size"`
	ReceivedAt time.Time         `json:"received_at"`
}

func (a *API) createWebhookInbox(w http.ResponseWriter, r *http.Request) {
	id, err := newUUID()
	if err != nil {
		a.dbError(w, "generate webhook inbox ID", err)
		return
	}
	token, err := randomHex(32)
	if err != nil {
		a.dbError(w, "generate webhook receive token", err)
		return
	}
	inbox := webhookInbox{ID: id, Environment: requestEnvironment(r.Context()),
		ReceivePath: "/webhook-inbox/" + token}
	err = a.Store.DB.QueryRowContext(r.Context(), `INSERT INTO webhook_inboxes
		(id,client_id,environment,receive_token) VALUES($1,$2,$3,$4) RETURNING created_at`,
		id, clientID(r.Context()), inbox.Environment, token).Scan(&inbox.CreatedAt)
	if err != nil {
		a.dbError(w, "create webhook inbox", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": inbox})
}

func (a *API) getWebhookInbox(w http.ResponseWriter, r *http.Request) {
	var inbox webhookInbox
	var token string
	err := a.Store.DB.QueryRowContext(r.Context(), `SELECT id,environment,receive_token,created_at
		FROM webhook_inboxes WHERE id=$1 AND client_id=$2 AND environment=$3`,
		r.PathValue("id"), clientID(r.Context()), requestEnvironment(r.Context())).
		Scan(&inbox.ID, &inbox.Environment, &token, &inbox.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, 404, "webhook inbox not found")
		return
	}
	if err != nil {
		a.dbError(w, "load webhook inbox", err)
		return
	}
	inbox.ReceivePath = "/webhook-inbox/" + token
	receipts, err := a.webhookReceipts(r, inbox.ID)
	if err != nil {
		a.dbError(w, "list webhook receipts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inbox, "receipts": receipts})
}

func (a *API) webhookReceipts(r *http.Request, id string) ([]webhookReceipt, error) {
	rows, err := a.Store.DB.QueryContext(r.Context(), `SELECT id,LEFT(body,512),
		octet_length(body),received_at
		FROM webhook_receipts WHERE inbox_id=$1 ORDER BY id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	receipts := []webhookReceipt{}
	for rows.Next() {
		var receipt webhookReceipt
		if err := rows.Scan(&receipt.ID,
			&receipt.Preview, &receipt.Size, &receipt.ReceivedAt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, rows.Err()
}

func (a *API) getWebhookReceipt(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("receipt"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, 400, "receipt ID must be a positive integer")
		return
	}
	var receipt struct {
		webhookReceipt
		Body string `json:"body"`
	}
	err = a.Store.DB.QueryRowContext(r.Context(), `SELECT r.id,r.headers,LEFT(r.body,512),
		octet_length(r.body),r.body,r.received_at
		FROM webhook_receipts r JOIN webhook_inboxes i ON i.id=r.inbox_id
		WHERE r.id=$1 AND i.id=$2 AND i.client_id=$3 AND i.environment=$4`,
		id, r.PathValue("id"), clientID(r.Context()),
		requestEnvironment(r.Context())).Scan(&receipt.ID, jsonScanner{target: &receipt.Headers},
		&receipt.Preview, &receipt.Size, &receipt.Body, &receipt.ReceivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, 404, "webhook receipt not found")
		return
	}
	if err != nil {
		a.dbError(w, "load webhook receipt", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": receipt})
}

func (a *API) receiveWebhook(w http.ResponseWriter, r *http.Request) {
	var id string
	err := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT id FROM webhook_inboxes WHERE receive_token=$1`, r.PathValue("token")).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, 404, "webhook inbox not found")
		return
	}
	if err != nil {
		a.dbError(w, "find webhook receiver", err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		writeWebhookReadError(w, err)
		return
	}
	if !utf8.Valid(body) || strings.ContainsRune(string(body), 0) {
		writeError(w, http.StatusBadRequest, 400, "webhook body must be UTF-8 text without NUL bytes")
		return
	}
	if err := a.storeWebhookReceipt(r, id, body); err != nil {
		a.dbError(w, "store webhook receipt", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "received"})
}

func writeWebhookReadError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, 413, "webhook body exceeds 256 KiB")
		return
	}
	writeError(w, http.StatusBadRequest, 400, "could not read webhook request body; retry the request")
}

func (a *API) storeWebhookReceipt(r *http.Request, id string, body []byte) error {
	headers := webhookRequestHeaders(r)
	encoded, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return insertWebhookReceipt(r, tx, id, encoded, body)
}

func webhookRequestHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	names := []string{}
	sensitive := map[string]bool{"authorization": true, "cookie": true, "proxy-authorization": true}
	for name := range r.Header {
		if !sensitive[strings.ToLower(name)] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names[:min(len(names), 64)] {
		value := strings.Join(r.Header.Values(name), ", ")
		headers[name] = value[:min(len(value), 1024)]
	}
	return headers
}

func insertWebhookReceipt(r *http.Request, tx *sql.Tx, id string,
	headers, body []byte,
) error {
	var lockedID string
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM webhook_inboxes
		WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO webhook_receipts(inbox_id,headers,body)
		VALUES($1,$2,$3)`, id, headers, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM webhook_receipts WHERE inbox_id=$1
		AND id NOT IN (SELECT id FROM webhook_receipts WHERE inbox_id=$1
		ORDER BY id DESC LIMIT 100)`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func validateInbox(r *http.Request, tx *sql.Tx, id string) error {
	if id == "" {
		return nil
	}
	var exists bool
	err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM webhook_inboxes
		WHERE id=$1 AND client_id=$2 AND environment=$3)`,
		id, clientID(r.Context()), requestEnvironment(r.Context())).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return &httpError{Status: 404, Code: 404, Message: "webhook inbox not found"}
	}
	return nil
}
