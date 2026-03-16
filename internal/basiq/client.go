package basiq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/basiq-app/pkg/models"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string

	mu          sync.RWMutex
	token       string
	tokenExp    time.Time
	refreshMu   sync.Mutex
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
	}
}

func (c *Client) Authenticate(ctx context.Context) error {
	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/token", strings.NewReader("scope=SERVER_ACCESS"))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Basic "+c.apiKey)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("basiq-version", "2.1")
		return req, nil
	}

	resp, err := c.doWithRetry(ctx, buildReq)
	if err != nil {
		return fmt.Errorf("authenticating: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp models.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	c.mu.Lock()
	c.token = tokenResp.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	c.mu.Unlock()

	return nil
}

func (c *Client) getToken(_ context.Context) (string, error) {
	c.mu.RLock()
	token := c.token
	exp := c.tokenExp
	c.mu.RUnlock()

	if time.Until(exp) >= 5*time.Minute {
		return token, nil
	}

	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.mu.RLock()
	token = c.token
	exp = c.tokenExp
	c.mu.RUnlock()

	if time.Until(exp) >= 5*time.Minute {
		return token, nil
	}

	refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Authenticate(refreshCtx); err != nil {
		return "", fmt.Errorf("refreshing token: %w", err)
	}

	c.mu.RLock()
	token = c.token
	c.mu.RUnlock()

	return token, nil
}

func (c *Client) CreateUser(ctx context.Context, email string) (*models.UserResponse, error) {
	payload, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return nil, fmt.Errorf("marshaling user payload: %w", err)
	}

	buildReq := func() (*http.Request, error) {
		token, err := c.getToken(ctx)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/users", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	resp, err := c.doWithRetry(ctx, buildReq)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create user failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var user models.UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding user response: %w", err)
	}

	return &user, nil
}

type connectionRequest struct {
	Institution struct {
		ID string `json:"id"`
	} `json:"institution"`
	LoginID  string `json:"loginId"`
	Password string `json:"password"`
}

func (c *Client) CreateConnection(ctx context.Context, userID, institutionID, loginID, password string) (*models.JobResponse, error) {
	connReq := connectionRequest{
		LoginID:  loginID,
		Password: password,
	}
	connReq.Institution.ID = institutionID

	payload, err := json.Marshal(connReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling connection payload: %w", err)
	}

	connURL := c.baseURL + "/users/" + userID + "/connections"

	buildReq := func() (*http.Request, error) {
		token, err := c.getToken(ctx)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, connURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	resp, err := c.doWithRetry(ctx, buildReq)
	if err != nil {
		return nil, fmt.Errorf("creating connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create connection failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var job models.JobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decoding connection job response: %w", err)
	}

	return &job, nil
}

const maxJobPollAttempts = 60

func (c *Client) WaitForJob(ctx context.Context, jobID string) (*models.JobResponse, error) {
	jobURL := c.baseURL + "/jobs/" + jobID

	for attempt := 0; attempt < maxJobPollAttempts; attempt++ {
		buildReq := func() (*http.Request, error) {
			token, err := c.getToken(ctx)
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		}

		resp, err := c.doWithRetry(ctx, buildReq)
		if err != nil {
			return nil, fmt.Errorf("polling job: %w", err)
		}

		var job models.JobResponse
		if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding job response: %w", err)
		}
		resp.Body.Close()

		allDone := true
		for _, step := range job.Steps {
			if step.Status == "failed" {
				return &job, fmt.Errorf("job step %q failed", step.Title)
			}
			if step.Status != "success" {
				allDone = false
			}
		}

		if allDone {
			return &job, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	return nil, fmt.Errorf("job %s did not complete after %d attempts", jobID, maxJobPollAttempts)
}

func (c *Client) GetTransactions(ctx context.Context, userID string) ([]models.Transaction, error) {
	return c.getTransactions(ctx, userID, "")
}

func (c *Client) GetTransactionsSince(ctx context.Context, userID, sinceDate string) ([]models.Transaction, error) {
	return c.getTransactions(ctx, userID, sinceDate)
}

func (c *Client) getTransactions(ctx context.Context, userID, sinceDate string) ([]models.Transaction, error) {
	var allTransactions []models.Transaction
	nextURL := c.baseURL + "/users/" + userID + "/transactions?limit=500"
	if sinceDate != "" {
		nextURL += "&filter=" + url.QueryEscape("transaction.postDate.gteq('"+sinceDate+"')")
	}

	for nextURL != "" {
		currentURL := nextURL
		buildReq := func() (*http.Request, error) {
			token, err := c.getToken(ctx)
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		}

		resp, err := c.doWithRetry(ctx, buildReq)
		if err != nil {
			return nil, fmt.Errorf("fetching transactions: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("get transactions failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var txList models.TransactionList
		if err := json.NewDecoder(resp.Body).Decode(&txList); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding transactions response: %w", err)
		}
		resp.Body.Close()

		allTransactions = append(allTransactions, txList.Data...)

		next := txList.Links.Next
		if next != "" && !strings.HasPrefix(next, "http") {
			next = c.baseURL + next
		}
		nextURL = next
	}

	return allTransactions, nil
}

func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) (*http.Response, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := buildReq()
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}
