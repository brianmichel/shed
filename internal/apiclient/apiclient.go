// Package apiclient is a thin HTTP client over Shed's public /v1 API,
// used by the CLI so it drives shed the same way any other caller would.
package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/brianmichel/shed/internal/api"
	"github.com/brianmichel/shed/internal/model"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// APIError is returned when the server responds with a non-2xx status.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (status %d, code %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("request failed (status %d)", e.Status)
}

type CreateJobRequest struct {
	Repo         string            `json:"repo"`
	BaseRef      string            `json:"base_ref,omitempty"`
	WorkBranch   string            `json:"work_branch,omitempty"`
	Prompt       string            `json:"prompt"`
	ComputeClass string            `json:"compute_class,omitempty"`
	AgentDriver  string            `json:"agent_driver,omitempty"`
	ScmDriver    string            `json:"scm_driver,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (c *Client) CreateJob(ctx context.Context, in CreateJobRequest) (model.Job, error) {
	var out struct {
		Data model.Job `json:"data"`
	}
	body, err := json.Marshal(in)
	if err != nil {
		return model.Job{}, err
	}
	if err := c.do(ctx, http.MethodPost, "/v1/jobs", bytes.NewReader(body), &out); err != nil {
		return model.Job{}, err
	}
	return out.Data, nil
}

func (c *Client) ListJobs(ctx context.Context) ([]model.Job, error) {
	var out struct {
		Data []model.Job `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/jobs", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) GetJob(ctx context.Context, jobID string) (model.Job, error) {
	var out struct {
		Data model.Job `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/"+jobID, nil, &out); err != nil {
		return model.Job{}, err
	}
	return out.Data, nil
}

func (c *Client) CancelJob(ctx context.Context, jobID string) (model.Job, error) {
	var out struct {
		Data model.Job `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/cancel", nil, &out); err != nil {
		return model.Job{}, err
	}
	return out.Data, nil
}

// StreamJobEvents makes one SSE-shaped request for events after the given
// cursor, invoking onEvent for each event currently available, and returns
// the cursor to resume from. The server does not hold the connection open
// past what is currently buffered, so following a live job means calling
// this repeatedly with the returned cursor.
func (c *Client) StreamJobEvents(ctx context.Context, jobID string, after int64, onEvent func(model.Event)) (int64, error) {
	url := fmt.Sprintf("%s/v1/jobs/%s/events?after=%d", c.BaseURL, jobID, after)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return after, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return after, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return after, apiErrorFromBody(resp)
	}
	next := after
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		raw := strings.Join(dataLines, "\n")
		dataLines = nil
		var ev model.Event
		if err := json.Unmarshal([]byte(raw), &ev); err == nil {
			onEvent(ev)
			if ev.Seq > next {
				next = ev.Seq
			}
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
	return next, scanner.Err()
}

func (c *Client) do(ctx context.Context, method, path string, body *bytes.Reader, out any) error {
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = body
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiErrorFromBody(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func apiErrorFromBody(resp *http.Response) error {
	var body api.ErrorBody
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return &APIError{Status: resp.StatusCode, Code: body.Error.Code, Message: body.Error.Message}
}
