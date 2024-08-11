package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func createCustomHTTPClient(userAgent string, validate bool, httpTimeout string) http.Client {
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: validate,
		},
	}

	customTimeout, err := time.ParseDuration(httpTimeout)
	if err != nil {
		slog.Error(fmt.Sprintf("Unable to parse HTTP Timeout value: %s", httpTimeout))
		os.Exit(1)
	}

	// Create a custom http.Client
	client := &http.Client{
		Timeout:   customTimeout,
		Transport: transport,
	}

	// Set the User-Agent header globally for this client
	client.Transport = &customTransport{
		Transport: transport,
		UserAgent: userAgent,
	}

	return *client
}

// customTransport is a custom http.RoundTripper that sets the User-Agent header
type customTransport struct {
	Transport http.RoundTripper
	UserAgent string
}

func (t *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.UserAgent)
	return t.Transport.RoundTrip(req)
}
