package testing

import (
	"crypto/tls"
	"net/http"
)

// IsServerHealth returns true if the server is healthy
func IsServerHealth(endpoint string, insecure bool) bool {
	var httpClient = http.DefaultClient
	if insecure {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}
	resp, err := httpClient.Get(endpoint)
	if err == nil {
		resp.Body.Close()
	}
	return err == nil && resp.StatusCode == http.StatusOK
}
