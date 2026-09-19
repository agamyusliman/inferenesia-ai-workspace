package core

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPlaygroundImageRejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{
		"http://example.com/image.png", "https://localhost/image.png",
		"https://127.0.0.1/image.png", "https://[::1]/image.png",
		"https://10.0.0.1/image.png", "https://169.254.169.254/latest/meta-data/",
		"https://[::ffff:127.0.0.1]/image.png", "https://100.64.0.1/image.png",
		"https://user:password@example.com/image.png", "https://example.com:4110/image.png",
		"file:///etc/passwd",
	} {
		t.Run(target, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/playground/image", strings.NewReader(`{"url":"`+target+`"}`))
			w := httptest.NewRecorder()
			servePlaygroundImage(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("unsafe target returned %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPlaygroundImageRejectsPrivateResolvedAddresses(t *testing.T) {
	for _, address := range []string{"::1", "::ffff:192.168.1.1", "fd00::1", "fe80::1", "127.0.0.1", "10.2.3.4", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "0.0.0.0", "224.0.0.1"} {
		if publicImageAddress(netip.MustParseAddr(address)) {
			t.Errorf("accepted unsafe address %s", address)
		}
	}
	if !publicImageAddress(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
	if _, err := validateImageURL("https://images.example.com/a.png?signature=abc"); err != nil {
		t.Fatal(err)
	}
}
