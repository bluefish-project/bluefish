package rvfs

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client handles HTTP communication with Redfish endpoint
type Client struct {
	endpoint string
	token    string
	username string
	password string
	http     *http.Client
}

// NewClient creates and authenticates a Redfish client
func NewClient(endpoint, username, password string, insecure bool) (*Client, error) {
	// Parse endpoint to validate
	_, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	// Create HTTP client with optional TLS verification
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		},
	}

	client := &Client{
		endpoint: endpoint,
		username: username,
		password: password,
		http:     httpClient,
	}

	// Authenticate
	if err := client.Login(); err != nil {
		return nil, err
	}

	return client, nil
}

// Login performs session-based authentication
func (c *Client) Login() error {
	loginURL := c.endpoint + "/redfish/v1/SessionService/Sessions"

	payload := map[string]string{
		"UserName": c.username,
		"Password": c.password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &NetworkError{Path: "/SessionService/Sessions", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return &HTTPError{Path: "/SessionService/Sessions", StatusCode: resp.StatusCode}
	}

	// Extract session token from header
	c.token = resp.Header.Get("X-Auth-Token")
	if c.token == "" {
		// Some implementations use Location header
		location := resp.Header.Get("Location")
		if location != "" {
			c.token = "session-based"
		}
	}

	return nil
}

// Logout closes the session
func (c *Client) Logout() error {
	// Session logout implementation would go here
	// For now, just clear the token
	c.token = ""
	return nil
}

// Fetch retrieves raw JSON from a path
func (c *Client) Fetch(path string) ([]byte, error) {
	// Normalize path
	if path[0] != '/' {
		path = "/" + path
	}

	url := c.endpoint + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("X-Auth-Token", c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &NetworkError{Path: path, Err: err}
	}
	defer resp.Body.Close()

	// Handle 401 Unauthorized - session may have expired
	if resp.StatusCode == http.StatusUnauthorized {
		// Attempt to re-authenticate
		if err := c.Login(); err != nil {
			return nil, &HTTPError{Path: path, StatusCode: resp.StatusCode}
		}

		// Retry the request with new token
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		if c.token != "" {
			req.Header.Set("X-Auth-Token", c.token)
		}
		req.Header.Set("Accept", "application/json")

		resp, err = c.http.Do(req)
		if err != nil {
			return nil, &NetworkError{Path: path, Err: err}
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{Path: path, StatusCode: resp.StatusCode}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &NetworkError{Path: path, Err: err}
	}

	return data, nil
}
