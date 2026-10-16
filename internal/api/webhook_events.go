package api

import (
	"context"
	"fmt"
)

func (a *API) webhookDestination(db webhookExecutor, ctx context.Context,
	orderID any, owner string,
) (string, map[string]any, error) {
	var order map[string]any
	var token string
	err := db.QueryRowContext(ctx, `SELECT o.payload,COALESCE(i.receive_token,'')
		FROM orders o LEFT JOIN webhook_inboxes i
		ON i.id=(o.payload->>'webhook_inbox_id') AND i.client_id=o.client_id
		AND i.environment=o.environment WHERE o.id=$1 AND o.client_id=$2`,
		orderID, owner).Scan(jsonScanner{target: &order}, &token)
	if err != nil {
		return "", nil, fmt.Errorf("load webhook destination for order %v: %w", orderID, err)
	}
	if id, _ := order["webhook_inbox_id"].(string); id != "" {
		if token == "" {
			return "", nil, fmt.Errorf("webhook inbox for order %v no longer exists", orderID)
		}
		return "/webhook-inbox/" + token, order, nil
	}
	return a.Config.WebhookURL, order, nil
}

func webhookEventData(data, order map[string]any) map[string]any {
	result := make(map[string]any, len(data)+3)
	for key, value := range data {
		result[key] = value
	}
	if simulation := order["simulation"]; simulation != nil {
		result["simulation"] = simulation
	}
	if driver, ok := order["driver"].(map[string]any); ok {
		if id, _ := driver["id"].(string); id != "" {
			result["driver_id"] = id
		}
	}
	if order["status"] == "CANCELLED" {
		result["cancel_code"] = order["cancel_code"]
	}
	return result
}
