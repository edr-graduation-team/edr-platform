package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func sanitizeIPToken(ip string) string {
	return strings.NewReplacer(":", "_", ".", "_", "/", "_").Replace(ip)
}

func (h *Handler) blockIP(ctx context.Context, params map[string]string) (string, error) {
	ip := strings.TrimSpace(params["ip"])
	if ip == "" {
		return "", fmt.Errorf("ip parameter is required")
	}
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid ip address: %s", ip)
	}
	dir := strings.ToLower(strings.TrimSpace(params["direction"]))
	if dir == "" {
		dir = "both"
	}
	var dirs []string
	switch dir {
	case "in":
		dirs = []string{"in"}
	case "out":
		dirs = []string{"out"}
	case "both":
		dirs = []string{"in", "out"}
	default:
		return "", fmt.Errorf("direction must be in, out, or both")
	}
	tok := sanitizeIPToken(ip)
	var parts []string
	for _, d := range dirs {
		name := fmt.Sprintf("EDR_BLOCK_IP_%s_%s", d, tok)
		_ = exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).Run()
		args := []string{"advfirewall", "firewall", "add", "rule",
			"name=" + name, "dir=" + d, "action=block", "remoteip=" + ip, "protocol=any", "enable=yes"}
		out, err := exec.CommandContext(ctx, "netsh", args...).CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("netsh add rule %s: %w", name, err)
		}
		parts = append(parts, name)
	}
	return fmt.Sprintf("Firewall BLOCK rules installed: %s", strings.Join(parts, ", ")), nil
}

func (h *Handler) unblockIP(ctx context.Context, params map[string]string) (string, error) {
	ip := strings.TrimSpace(params["ip"])
	if ip == "" {
		return "", fmt.Errorf("ip parameter is required")
	}
	tok := sanitizeIPToken(ip)
	var removed []string
	for _, d := range []string{"in", "out"} {
		name := fmt.Sprintf("EDR_BLOCK_IP_%s_%s", d, tok)
		out, _ := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).CombinedOutput()
		if !strings.Contains(strings.ToLower(string(out)), "no rules matched") &&
			!strings.Contains(strings.ToLower(string(out)), "deleted") &&
			!strings.Contains(strings.ToLower(string(out)), "ok") {
			// netsh returns ok on success; treat non-fatal
		}
		removed = append(removed, name)
	}
	return fmt.Sprintf("Removed block rules (if present): %s", strings.Join(removed, ", ")), nil
}

func (h *Handler) blockDomain(ctx context.Context, params map[string]string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(params["domain"]))
	if domain == "" {
		return "", fmt.Errorf("domain parameter is required")
	}
	hosts := `C:\Windows\System32\drivers\etc\hosts`
	data, err := os.ReadFile(hosts)
	if err != nil {
		return "", fmt.Errorf("read hosts: %w", err)
	}
	markerBegin := "# EDR_BLOCK_BEGIN " + domain + "\n"
	markerEnd := "# EDR_BLOCK_END " + domain + "\n"
	block := markerBegin + "127.0.0.1 " + domain + "\n" + markerEnd
	if bytes.Contains(data, []byte(markerBegin)) {
		return "Domain already blocked in hosts file", nil
	}
	f, err := os.OpenFile(hosts, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("open hosts for append: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + block); err != nil {
		return "", fmt.Errorf("write hosts: %w", err)
	}
	return fmt.Sprintf("Hosts sinkhole added for domain %s", domain), nil
}

func (h *Handler) unblockDomain(ctx context.Context, params map[string]string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(params["domain"]))
	if domain == "" {
		return "", fmt.Errorf("domain parameter is required")
	}
	hosts := `C:\Windows\System32\drivers\etc\hosts`
	data, err := os.ReadFile(hosts)
	if err != nil {
		return "", fmt.Errorf("read hosts: %w", err)
	}
	markerBegin := "# EDR_BLOCK_BEGIN " + domain
	markerEnd := "# EDR_BLOCK_END " + domain
	lines := strings.Split(string(data), "\n")
	var out []string
	inBlock := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, markerBegin) {
			inBlock = true
			continue
		}
		if strings.HasPrefix(t, markerEnd) {
			inBlock = false
			continue
		}
		if inBlock {
			continue
		}
		out = append(out, line)
	}
	newData := strings.Join(out, "\n")
	if err := os.WriteFile(hosts, []byte(strings.TrimSpace(newData)+"\n"), 0644); err != nil {
		return "", fmt.Errorf("write hosts: %w", err)
	}
	return fmt.Sprintf("Hosts block removed for domain %s", domain), nil
}

func (h *Handler) updateSignatures(ctx context.Context, params map[string]string) (string, error) {
	h.mu.Lock()
	st := h.sigStore
	h.mu.Unlock()
	if st == nil {
		return "", fmt.Errorf("signature store not initialized on this agent")
	}
	rawURL := strings.TrimSpace(params["url"])
	want := strings.ToLower(strings.TrimSpace(params["checksum_sha256"]))
	if rawURL == "" {
		return "", fmt.Errorf("url parameter is required")
	}
	if !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		return "", fmt.Errorf("url must use HTTPS")
	}
	force := strings.EqualFold(params["force"], "true")
	format := strings.ToLower(strings.TrimSpace(params["format"]))
	csvFeed := format == "csv" || strings.Contains(strings.ToLower(rawURL), "bazaar.abuse.ch/export/csv")
	// Server-managed delta feed: checksum is meaningless because the response changes
	// on every call (it returns only the delta since the agent's last version).
	serverFeed := strings.Contains(rawURL, "signatures/feed.ndjson")
	if !csvFeed && !serverFeed && want == "" {
		return "", fmt.Errorf("checksum_sha256 is required for NDJSON feeds (or use format=csv with MalwareBazaar CSV URL)")
	}

	// For the server's NDJSON feed, automatically append the local since_version so the
	// command benefits from delta sync without the dashboard needing to know the cursor.
	finalURL := rawURL
	if strings.Contains(rawURL, "signatures/feed.ndjson") {
		if localVer, verErr := st.GetServerVersion(); verErr == nil {
			if parsed, parseErr := neturl.Parse(rawURL); parseErr == nil {
				q := parsed.Query()
				if q.Get("since_version") == "" {
					q.Set("since_version", strconv.FormatInt(localVer, 10))
					parsed.RawQuery = q.Encode()
					finalURL = parsed.String()
				}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, finalURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return "", err
	}
	if want != "" {
		sum := sha256.Sum256(body)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return "", fmt.Errorf("checksum mismatch: expected %s got %s", want, got)
		}
	}

	var inserted, skipped int
	if csvFeed {
		limit := 0
		if v := strings.TrimSpace(params["hash_limit"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		inserted, skipped, err = st.MergeFromMalwareBazaarCSV(bytes.NewReader(body), force, limit)
		if err != nil {
			return "", fmt.Errorf("merge MalwareBazaar CSV: %w", err)
		}
	} else {
		inserted, skipped, err = st.MergeFromNDJSON(bytes.NewReader(body), force)
		if err != nil {
			return "", fmt.Errorf("merge signatures: %w", err)
		}
		// For server-managed feed responses, persist the highest returned version so
		// future delta pulls continue from the correct cursor.
		if serverFeed {
			if maxVer := extractMaxVersionFromNDJSON(body); maxVer > 0 {
				_ = st.SetServerVersion(maxVer)
			}
		}
	}
	n, _ := st.Version()
	return fmt.Sprintf("UPDATE_SIGNATURES: inserted=%d skipped=%d db_version=v%d total_entries=%d", inserted, skipped, n, n), nil
}

func extractMaxVersionFromNDJSON(body []byte) int64 {
	var maxV int64
	dec := json.NewDecoder(bytes.NewReader(body))
	for dec.More() {
		var row struct {
			Version int64 `json:"version"`
		}
		if err := dec.Decode(&row); err != nil {
			break
		}
		if row.Version > maxV {
			maxV = row.Version
		}
	}
	return maxV
}
