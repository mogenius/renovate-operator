package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func testPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func TestListInstallationsPaginatesAndMapsFields(t *testing.T) {
	const total = installationsPerPage + 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		start := (page - 1) * installationsPerPage
		end := min(start+installationsPerPage, total)
		items := make([]string, 0, installationsPerPage)
		for id := start; id < end; id++ {
			suspended := "null"
			if id == 0 {
				suspended = `"2026-01-01T00:00:00Z"`
			}
			items = append(items, fmt.Sprintf(`{"id":%d,"account":{"login":"acct-%d","type":"Organization"},"suspended_at":%s}`, id, id, suspended))
		}
		_, _ = fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	}))
	defer srv.Close()

	got, err := NewInstallationLister(srv.Client()).ListInstallations(context.Background(), "42", testPEM(t), srv.URL)
	if err != nil {
		t.Fatalf("ListInstallations failed: %v", err)
	}
	if len(got) != total {
		t.Fatalf("expected %d installations across pages, got %d", total, len(got))
	}
	if !got[0].Suspended || got[1].Suspended {
		t.Error("suspended_at must map to Suspended")
	}
	if got[total-1].Account != fmt.Sprintf("acct-%d", total-1) || got[total-1].AccountType != "Organization" {
		t.Errorf("unexpected last installation %#v", got[total-1])
	}
}

func TestListInstallationsSkipsEnterpriseAccounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":1,"account":{"login":"alice","type":"User"},"suspended_at":null},
			{"id":2,"account":{"slug":"acme","name":"Acme"},"suspended_at":null},
			{"id":3,"account":null,"suspended_at":null}
		]`))
	}))
	defer srv.Close()

	got, err := NewInstallationLister(srv.Client()).ListInstallations(context.Background(), "42", testPEM(t), srv.URL)
	if err != nil {
		t.Fatalf("ListInstallations failed: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("expected only the user installation, got %#v", got)
	}
}

func TestListInstallationsReportsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	_, err := NewInstallationLister(srv.Client()).ListInstallations(context.Background(), "42", testPEM(t), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("expected the GitHub error to surface, got %v", err)
	}
}
