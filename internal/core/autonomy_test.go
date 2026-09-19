package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAutonomyHTTPRejectsInvalidWithoutChangingPermissions(t *testing.T) {
	svc, err := NewService(Options{ConfigHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.SetAutonomy("off"); err != nil {
		t.Fatal(err)
	}
	h := svc.Handler()
	for _, body := range []string{`{"level":"invalid"}`, `{}`} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/autonomy", strings.NewReader(body)))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid setting status = %d, want 400: %s", rr.Code, rr.Body.String())
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/autonomy", nil))
	var view AutonomyView
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Level != "off" {
		t.Fatalf("invalid input changed permissions to %q", view.Level)
	}
}
