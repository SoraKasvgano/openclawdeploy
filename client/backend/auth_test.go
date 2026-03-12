package backend

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientLocalAuthProtectsClientAPI(t *testing.T) {
	app := &App{
		cfg:    &Config{WebUsername: "admin", WebPassword: "admin"},
		logger: log.New(io.Discard, "", 0),
		syncer: NewSyncer(func() Config { return Config{} }, nil),
	}

	server := httptest.NewServer(app.routes())
	defer server.Close()

	unauthorizedResp, err := http.Get(server.URL + "/api/v1/client/status")
	if err != nil {
		t.Fatalf("unauthorized request failed: %v", err)
	}
	defer unauthorizedResp.Body.Close()
	if unauthorizedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected unauthorized status: %d", unauthorizedResp.StatusCode)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := server.Client()
	client.Jar = jar
	loginResp, err := client.Post(server.URL+"/api/v1/client/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"admin"}`))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected login status: %d", loginResp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(loginResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body["username"] != "admin" {
		t.Fatalf("unexpected login username: %s", body["username"])
	}

	meResp, err := client.Get(server.URL + "/api/v1/client/auth/me")
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected me status: %d", meResp.StatusCode)
	}
}
