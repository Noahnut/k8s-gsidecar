package notifier

import (
	"bytes"
	"fmt"
	"k8s-gsidecar/logger"
	"log/slog"
	"net/http"
)

var l *slog.Logger = logger.GetLogger()

type BasicAuth struct {
	Username string
	Password string
}

type HTTPNotifier struct {
	URL       string
	Method    string
	BasicAuth *BasicAuth
	Payload   string
}

func NewHTTPNotifier(
	URL string,
	Method string,
	BasicAuth *BasicAuth,
	Payload string,
) *HTTPNotifier {
	return &HTTPNotifier{
		URL:       URL,
		Method:    Method,
		BasicAuth: BasicAuth,
		Payload:   Payload,
	}
}

func (n *HTTPNotifier) Notify() error {
	client := &http.Client{}

	httpMethodName := http.MethodGet

	if n.Method == http.MethodPost {
		httpMethodName = http.MethodPost
	}

	req, err := http.NewRequest(httpMethodName, n.URL, bytes.NewBufferString(n.Payload))
	if err != nil {
		l.Error("Failed to create HTTP request", "error", err)
		return err
	}

	if n.BasicAuth != nil {
		req.SetBasicAuth(n.BasicAuth.Username, n.BasicAuth.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		l.Error("Failed to notify", "status", resp.Status)
		return fmt.Errorf("failed to notify: %s", resp.Status)
	}

	return nil
}
