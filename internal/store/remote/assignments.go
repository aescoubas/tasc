package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/store"
)

func (c *Client) ForActor(actor string) store.Store {
	copy := *c
	copy.baseURL += "/actors/" + url.PathEscape(actor)
	return &copy
}

func (c *Client) Assignments() ([]store.AssignmentRow, error) {
	resp, err := c.http.Get(c.baseURL + "/assignments")
	if err != nil {
		return nil, err
	}
	return decodeAssignments(resp)
}

func (c *Client) ChangeAssignments(change store.AssignmentChange) ([]store.AssignmentRow, error) {
	body, err := json.Marshal(change)
	if err != nil {
		return nil, err
	}
	// A broken remote connection must not strand the lease worker in a single renewal.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/assignments", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	return decodeAssignments(resp)
}

func decodeAssignments(resp *http.Response) ([]store.AssignmentRow, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusGone {
			return nil, store.ErrLeaseExpired
		}
		return nil, fmt.Errorf("assignments: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var rows []store.AssignmentRow
	err := json.NewDecoder(resp.Body).Decode(&rows)
	return rows, err
}
