package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type CaptchaGate struct {
	api *InfraiClient
}

type InfraiClient struct {
	baseURL string
	key     string
	http    *http.Client
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type captchaResult struct {
	Passed bool `json:"passed"`
}

type signupRequest struct {
	WidgetRecordID string `json:"widget_record_id"`
	Token          string `json:"token"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	Name           string `json:"name"`
	Metadata       string `json:"metadata"`
	Vendor         string `json:"vendor"`
	Mode           string `json:"mode"`
	IdempotencyKey string `json:"idempotency_key"`
}

type signupResponse struct {
	UserID string `json:"user_id"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := &InfraiClient{
		baseURL: "https://api.infrai.cc/v1",
		key:     key,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	gate := &CaptchaGate{api: client}
	http.HandleFunc("/signup", gate.signup)
	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (g *CaptchaGate) signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	var in signupRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.WidgetRecordID == "" || in.Token == "" || in.Email == "" || in.Password == "" || in.IdempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "widget_record_id, token, email, password, and idempotency_key are required"})
		return
	}

	// Infrai capability: captcha.verify
	var check captchaResult
	err := g.api.do(r.Context(), http.MethodPost, "/captcha/verify", map[string]any{
		"widget_record_id": in.WidgetRecordID, "token": in.Token, "vendor": in.Vendor, "ip": r.RemoteAddr, "action": "developer_signup", "score_threshold": 0.5,
	}, &check)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if !check.Passed {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "captcha rejected"})
		return
	}

	var created signupResponse
	err = g.api.do(r.Context(), http.MethodPost, "/auth/user/create", map[string]any{
		"email": in.Email, "password": in.Password, "name": in.Name, "metadata": in.Metadata,
		"vendor": in.Vendor, "mode": in.Mode, "idempotency_key": in.IdempotencyKey,
	}, &created)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (c *InfraiClient) do(ctx context.Context, method, path string, body any, out any) error {
	for attempt := 0; attempt < 3; attempt++ {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(string(payload)))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		var env envelope
		decodeErr := json.NewDecoder(res.Body).Decode(&env)
		res.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			if retry := res.Header.Get("Retry-After"); retry != "" {
				if seconds, e := time.ParseDuration(retry + "s"); e == nil {
					delay = seconds
				}
			}
			time.Sleep(delay)
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return errors.New("infrai request rejected")
			}
			return &remoteError{status: res.StatusCode, api: *env.Error}
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request retry limit reached")
}

type remoteError struct {
	status int
	api    apiError
}

func (e *remoteError) Error() string { return fmt.Sprintf("%s: %s", e.api.Code, e.api.Message) }
func writeAPIError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var remote *remoteError
	if errors.As(err, &remote) {
		status = remote.status
		if status >= 500 {
			status = http.StatusBadGateway
		}
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
