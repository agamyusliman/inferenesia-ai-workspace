package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const playgroundImageLimit = 25 << 20

func publicImageAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() && !ip.IsUnspecified() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(ip) &&
		!netip.MustParsePrefix("192.0.0.0/24").Contains(ip) &&
		!netip.MustParsePrefix("198.18.0.0/15").Contains(ip)
}

func validateImageURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return nil, fmt.Errorf("image URL must be public HTTPS without credentials or a custom port")
	}
	if strings.EqualFold(u.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".localhost") {
		return nil, fmt.Errorf("local image hosts are not allowed")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicImageAddress(ip) {
		return nil, fmt.Errorf("private image addresses are not allowed")
	}
	return u, nil
}

// Resolve and dial the checked address, rather than resolving it again in the
// transport. Redirects undergo the same checks; provider keys are never sent.
var playgroundImageClient = &http.Client{
	Timeout: 45 * time.Second,
	Transport: &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || port != "443" {
				return nil, fmt.Errorf("image connection must use HTTPS port 443")
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("image host could not be resolved")
			}
			for _, ip := range ips {
				if !publicImageAddress(ip) {
					return nil, fmt.Errorf("image host resolves to a private or reserved address")
				}
			}
			dialer := net.Dialer{Timeout: 10 * time.Second}
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				err = dialErr
			}
			return nil, err
		},
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many image redirects")
		}
		_, err := validateImageURL(req.URL.String())
		return err
	},
}

func servePlaygroundImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid image download request"))
		return
	}
	u, err := validateImageURL(strings.TrimSpace(body.URL))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid image URL"))
		return
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp,image/gif")
	res, err := playgroundImageClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("image host could not be reached"))
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("image host returned HTTP %d", res.StatusCode))
		return
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, playgroundImageLimit+1))
	if err != nil || len(data) == 0 || len(data) > playgroundImageLimit {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("image download failed or exceeds 25 MB"))
		return
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		writeErr(w, http.StatusBadGateway, fmt.Errorf("image host did not return a supported raster image"))
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "attachment")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
