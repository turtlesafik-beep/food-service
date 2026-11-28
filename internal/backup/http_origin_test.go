package backup

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func crossSiteRequest(host, origin, forwardedProto string, secure bool) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "http://example.test/api/admin/backups", nil)
	request.Host = host
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if forwardedProto != "" {
		request.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	if secure {
		request.TLS = &tls.ConnectionState{}
	}
	return request
}

func TestRejectCrossSiteComparesFullOrigin(t *testing.T) {
	tests := []struct {
		name          string
		publicBaseURL string
		host          string
		origin        string
		forwarded     string
		secure        bool
		secFetchSite  string
		wantReject    bool
	}{
		{
			name:   "local direct same origin",
			host:   "localhost:3000",
			origin: "http://localhost:3000",
		},
		{
			name:   "local default port",
			host:   "localhost",
			origin: "http://localhost",
		},
		{
			name:   "local loopback",
			host:   "127.0.0.1:3000",
			origin: "http://127.0.0.1:3000",
		},
		{
			name:      "production behind proxy",
			host:      "sdahub.ru",
			origin:    "https://sdahub.ru",
			forwarded: "https",
		},
		{
			name:      "production explicit default port",
			host:      "sdahub.ru",
			origin:    "https://sdahub.ru:443",
			forwarded: "https",
		},
		{
			name:   "production direct TLS",
			host:   "sdahub.ru",
			origin: "https://sdahub.ru",
			secure: true,
		},
		{
			name:   "request without origin",
			host:   "localhost:3000",
			origin: "",
		},
		{
			name:       "different port",
			host:       "localhost:3000",
			origin:     "http://localhost:3001",
			wantReject: true,
		},
		{
			name:       "different scheme",
			host:       "localhost:3000",
			origin:     "https://localhost:3000",
			wantReject: true,
		},
		{
			name:       "production plain http origin",
			host:       "sdahub.ru",
			origin:     "http://sdahub.ru",
			forwarded:  "https",
			wantReject: true,
		},
		{
			name:       "foreign host",
			host:       "sdahub.ru",
			origin:     "https://evil.example",
			forwarded:  "https",
			wantReject: true,
		},
		{
			name:       "origin with userinfo",
			host:       "sdahub.ru",
			origin:     "https://user@sdahub.ru",
			forwarded:  "https",
			wantReject: true,
		},
		{
			name:       "origin with path",
			host:       "sdahub.ru",
			origin:     "https://sdahub.ru/admin",
			forwarded:  "https",
			wantReject: true,
		},
		{
			name:       "null origin",
			host:       "localhost:3000",
			origin:     "null",
			wantReject: true,
		},
		{
			name:       "non-http origin scheme",
			host:       "localhost:3000",
			origin:     "ftp://localhost:3000",
			wantReject: true,
		},
		{
			name:       "invalid forwarded scheme",
			host:       "localhost:3000",
			origin:     "http://localhost:3000",
			forwarded:  "ftp",
			wantReject: true,
		},
		{
			name:         "cross-site fetch metadata",
			host:         "localhost:3000",
			origin:       "http://localhost:3000",
			secFetchSite: "cross-site",
			wantReject:   true,
		},
		{
			name:          "configured production behind internal host",
			publicBaseURL: "https://sdahub.ru",
			host:          "app:3000",
			origin:        "https://sdahub.ru",
			forwarded:     "http",
		},
		{
			name:          "configured production with trailing slash",
			publicBaseURL: "https://sdahub.ru/",
			host:          "app:3000",
			origin:        "https://sdahub.ru",
		},
		{
			name:          "configured local development",
			publicBaseURL: "http://localhost:3000",
			host:          "localhost:3000",
			origin:        "http://localhost:3000",
		},
		{
			name:          "configured public rejects different port",
			publicBaseURL: "https://sdahub.ru",
			host:          "app:3000",
			origin:        "https://sdahub.ru:8443",
			wantReject:    true,
		},
		{
			name:          "configured public rejects plain http",
			publicBaseURL: "https://sdahub.ru",
			host:          "app:3000",
			origin:        "http://sdahub.ru",
			wantReject:    true,
		},
		{
			name:          "configured public rejects foreign origin",
			publicBaseURL: "https://sdahub.ru",
			host:          "app:3000",
			origin:        "https://evil.example",
			wantReject:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &Service{cfg: Config{PublicBaseURL: test.publicBaseURL}}
			request := crossSiteRequest(test.host, test.origin, test.forwarded, test.secure)
			if test.secFetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.secFetchSite)
			}
			recorder := httptest.NewRecorder()

			rejected := service.rejectCrossSite(recorder, request)
			if rejected != test.wantReject {
				t.Fatalf("rejected = %v, want %v", rejected, test.wantReject)
			}
			if rejected && recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
		})
	}
}
