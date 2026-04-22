package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

type iceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type iceConfigResponse struct {
	IceServers []iceServer `json:"iceServers"`
}

// IceConfig returns the ICE server configuration for WebRTC peer connections.
// Reads optional env vars:
//   - STUN_URLS      comma-separated STUN URLs (default: two public Google STUN servers)
//   - TURN_URLS      comma-separated TURN URLs (if unset, no TURN is advertised)
//   - TURN_USERNAME  TURN credential username
//   - TURN_PASSWORD  TURN credential password
//
// Operators behind symmetric NAT or Cloudflare proxy/tunnel must provide a working
// TURN server — STUN alone is insufficient on restrictive networks.
func (h *Handler) IceConfig(c echo.Context) error {
	servers := []iceServer{}

	stunEnv := strings.TrimSpace(os.Getenv("STUN_URLS"))
	if stunEnv == "" {
		servers = append(servers,
			iceServer{URLs: []string{"stun:stun.l.google.com:19302"}},
			iceServer{URLs: []string{"stun:stun1.l.google.com:19302"}},
		)
	} else {
		for _, u := range splitCSV(stunEnv) {
			servers = append(servers, iceServer{URLs: []string{u}})
		}
	}

	turnEnv := strings.TrimSpace(os.Getenv("TURN_URLS"))
	if turnEnv != "" {
		servers = append(servers, iceServer{
			URLs:       splitCSV(turnEnv),
			Username:   os.Getenv("TURN_USERNAME"),
			Credential: os.Getenv("TURN_PASSWORD"),
		})
	}

	return c.JSON(http.StatusOK, iceConfigResponse{IceServers: servers})
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
