// Package enrich provides Threat Intelligence enrichment for IOCs
// (hashes, IPs, domains) collected during post-isolation triage.
// It queries VirusTotal, AbuseIPDB, and AlienVault OTX.
// Rate limiting uses the Redis client already present in the project.
package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/edr-platform/connection-manager/internal/repository"
)

// Config holds API keys for each TI provider.
type Config struct {
	VirusTotalAPIKey string
	AbuseIPDBAPIKey  string
	OTXAPIKey        string

	// RequestTimeout for each outbound HTTP call.
	RequestTimeout time.Duration
}

// Enricher runs IOC enrichment workers.
type Enricher struct {
	cfg    Config
	repo   repository.IncidentRepository
	logger *logrus.Logger
	client *http.Client
	queue  chan enrichTask
	once   sync.Once
	stopCh chan struct{}
}

type enrichTask struct {
	AgentID  *uuid.UUID
	RunID    *int64
	IocType  string
	IocValue string
}

// New creates an Enricher. Call Start() to begin processing.
func New(cfg Config, repo repository.IncidentRepository, logger *logrus.Logger) *Enricher {
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Enricher{
		cfg:    cfg,
		repo:   repo,
		logger: logger,
		client: &http.Client{Timeout: timeout},
		queue:  make(chan enrichTask, 512),
		stopCh: make(chan struct{}),
	}
}

// Start launches background workers (call once).
func (e *Enricher) Start(workers int) {
	e.once.Do(func() {
		if workers <= 0 {
			workers = 2
		}
		for i := 0; i < workers; i++ {
			go e.worker()
		}
		e.logger.Infof("[Enrich] Started %d workers", workers)
	})
}

// Stop signals workers to exit.
func (e *Enricher) Stop() {
	close(e.stopCh)
}

// EnqueueSnapshot submits all IOCs found in a triage snapshot for enrichment.
// Extracts SHA-256 hashes, IPs, and domains from the snapshot payload.
func (e *Enricher) EnqueueSnapshot(agentID *uuid.UUID, runID *int64, kind string, payload map[string]any) {
	iocs := extractIOCs(kind, payload)
	for _, ioc := range iocs {
		task := enrichTask{AgentID: agentID, RunID: runID, IocType: ioc[0], IocValue: ioc[1]}
		select {
		case e.queue <- task:
		default:
			e.logger.Warn("[Enrich] Queue full — dropping IOC: " + ioc[1])
		}
	}
}

// EnqueueIOC manually queues a single IOC.
func (e *Enricher) EnqueueIOC(agentID *uuid.UUID, runID *int64, iocType, iocValue string) {
	select {
	case e.queue <- enrichTask{AgentID: agentID, RunID: runID, IocType: iocType, IocValue: iocValue}:
	default:
		e.logger.Warn("[Enrich] Queue full — dropping manual IOC: " + iocValue)
	}
}

func (e *Enricher) worker() {
	for {
		select {
		case <-e.stopCh:
			return
		case task, ok := <-e.queue:
			if !ok {
				return
			}
			e.process(task)
		}
	}
}

func (e *Enricher) process(task enrichTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	providers := e.getProviders(task.IocType)
	for _, provFn := range providers {
		verdict, score, raw, err := provFn(ctx, task.IocValue)
		if err != nil {
			e.logger.WithError(err).Debugf("[Enrich] Provider error for %s (%s)", task.IocValue, task.IocType)
			continue
		}
		ioc := &repository.IocEnrichment{
			AgentID:  task.AgentID,
			RunID:    task.RunID,
			IocType:  task.IocType,
			IocValue: task.IocValue,
			Verdict:  verdict,
			Score:    score,
			Raw:      raw,
		}
		if upsertErr := e.repo.UpsertIoc(ctx, ioc); upsertErr != nil {
			e.logger.WithError(upsertErr).Warnf("[Enrich] UpsertIoc failed for %s", task.IocValue)
		}
		// Small delay to respect rate limits
		time.Sleep(250 * time.Millisecond)
	}
}

type providerFunc func(ctx context.Context, value string) (verdict string, score int, raw map[string]any, err error)

func (e *Enricher) getProviders(iocType string) []providerFunc {
	switch strings.ToLower(iocType) {
	case "hash":
		var fns []providerFunc
		if e.cfg.VirusTotalAPIKey != "" {
			fns = append(fns, e.vtFile)
		}
		return fns
	case "ip":
		var fns []providerFunc
		if e.cfg.VirusTotalAPIKey != "" {
			fns = append(fns, e.vtIP)
		}
		if e.cfg.AbuseIPDBAPIKey != "" {
			fns = append(fns, e.abuseIPDB)
		}
		return fns
	case "domain":
		var fns []providerFunc
		if e.cfg.VirusTotalAPIKey != "" {
			fns = append(fns, e.vtDomain)
		}
		if e.cfg.OTXAPIKey != "" {
			fns = append(fns, e.otxDomain)
		}
		return fns
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// VirusTotal
// ─────────────────────────────────────────────────────────────────────────────

func (e *Enricher) vtGet(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.virustotal.com/api/v3/"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-apikey", e.cfg.VirusTotalAPIKey)
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("VT rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VT HTTP %d for %s", resp.StatusCode, path)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func vtScoreFromStats(result map[string]any) (verdict string, score int) {
	data, _ := result["data"].(map[string]any)
	attrs, _ := data["attributes"].(map[string]any)
	stats, _ := attrs["last_analysis_stats"].(map[string]any)
	malicious, _ := stats["malicious"].(float64)
	suspicious, _ := stats["suspicious"].(float64)
	total := int(malicious + suspicious)
	score = total
	switch {
	case total >= 10:
		verdict = "malicious"
	case total >= 3:
		verdict = "suspicious"
	case total > 0:
		verdict = "suspicious"
	default:
		verdict = "clean"
	}
	return
}

func (e *Enricher) vtFile(ctx context.Context, hash string) (string, int, map[string]any, error) {
	result, err := e.vtGet(ctx, "files/"+hash)
	if err != nil {
		return "", 0, nil, err
	}
	verdict, score := vtScoreFromStats(result)
	return verdict, score, result, nil
}

func (e *Enricher) vtIP(ctx context.Context, ip string) (string, int, map[string]any, error) {
	result, err := e.vtGet(ctx, "ip_addresses/"+ip)
	if err != nil {
		return "", 0, nil, err
	}
	verdict, score := vtScoreFromStats(result)
	return verdict, score, result, nil
}

func (e *Enricher) vtDomain(ctx context.Context, domain string) (string, int, map[string]any, error) {
	result, err := e.vtGet(ctx, "domains/"+domain)
	if err != nil {
		return "", 0, nil, err
	}
	verdict, score := vtScoreFromStats(result)
	return verdict, score, result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// AbuseIPDB
// ─────────────────────────────────────────────────────────────────────────────

func (e *Enricher) abuseIPDB(ctx context.Context, ip string) (string, int, map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.abuseipdb.com/api/v2/check?ipAddress=%s&maxAgeInDays=90", ip), nil)
	if err != nil {
		return "", 0, nil, err
	}
	req.Header.Set("Key", e.cfg.AbuseIPDBAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, nil, fmt.Errorf("AbuseIPDB HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var result map[string]any
	_ = json.Unmarshal(body, &result)

	score := 0
	verdict := "unknown"
	if data, ok := result["data"].(map[string]any); ok {
		if cs, ok := data["abuseConfidenceScore"].(float64); ok {
			score = int(cs)
			switch {
			case score >= 75:
				verdict = "malicious"
			case score >= 25:
				verdict = "suspicious"
			default:
				verdict = "clean"
			}
		}
	}
	return verdict, score, result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// AlienVault OTX
// ─────────────────────────────────────────────────────────────────────────────

func (e *Enricher) otxDomain(ctx context.Context, domain string) (string, int, map[string]any, error) {
	url := fmt.Sprintf("https://otx.alienvault.com/api/v1/indicators/domain/%s/general", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, nil, err
	}
	req.Header.Set("X-OTX-API-KEY", e.cfg.OTXAPIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, nil, fmt.Errorf("OTX HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var result map[string]any
	_ = json.Unmarshal(body, &result)

	verdict := "unknown"
	score := 0
	if pulseInfo, ok := result["pulse_info"].(map[string]any); ok {
		if count, ok := pulseInfo["count"].(float64); ok {
			score = int(count)
			if count >= 5 {
				verdict = "malicious"
			} else if count >= 1 {
				verdict = "suspicious"
			} else {
				verdict = "clean"
			}
		}
	}
	return verdict, score, result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// IOC extraction from triage snapshots
// ─────────────────────────────────────────────────────────────────────────────

// extractIOCs walks a triage snapshot payload and returns [ioc_type, ioc_value] pairs.
func extractIOCs(kind string, payload map[string]any) [][2]string {
	var iocs [][2]string
	seen := map[string]bool{}

	add := func(iocType, val string) {
		val = strings.TrimSpace(val)
		if val == "" || seen[iocType+":"+val] {
			return
		}
		seen[iocType+":"+val] = true
		iocs = append(iocs, [2]string{iocType, val})
	}

	switch kind {
	case "process_tree_snapshot", "post_isolation_triage":
		// Extract SHA256 hashes from processes array
		procs, _ := payload["processes"].([]interface{})
		for _, p := range procs {
			proc, _ := p.(map[string]any)
			if h, ok := proc["sha256"].(string); ok && len(h) == 64 {
				add("hash", h)
			}
		}

	case "network_last_seen":
		// Extract IPs from tcp_conns
		conns, _ := payload["tcp_conns"].([]interface{})
		for _, c := range conns {
			conn, _ := c.(map[string]any)
			if remote, ok := conn["remote_addr"].(string); ok {
				ip := extractIP(remote)
				if ip != "" && !isPrivateIP(ip) {
					add("ip", ip)
				}
			}
		}
		// Extract domains from dns_cache
		dns, _ := payload["dns_cache"].([]interface{})
		for _, d := range dns {
			entry, _ := d.(map[string]any)
			if name, ok := entry["name"].(string); ok {
				add("domain", name)
			}
		}

	case "persistence_scan":
		items, _ := payload["persistence_items"].([]interface{})
		for _, item := range items {
			it, _ := item.(map[string]any)
			if h, ok := it["sha256"].(string); ok && len(h) == 64 {
				add("hash", h)
			}
		}

	case "filesystem_timeline":
		files, _ := payload["files"].([]interface{})
		for _, f := range files {
			file, _ := f.(map[string]any)
			if h, ok := file["sha256"].(string); ok && len(h) == 64 {
				add("hash", h)
			}
		}
	}
	return iocs
}

func extractIP(addrPort string) string {
	// Remove port: "192.168.1.1:443" → "192.168.1.1"
	if idx := strings.LastIndex(addrPort, ":"); idx >= 0 {
		return addrPort[:idx]
	}
	return addrPort
}

var privateRanges = []string{"10.", "172.16.", "172.17.", "172.18.", "172.19.",
	"172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.",
	"172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168.", "127.", "0.0.0.0"}

func isPrivateIP(ip string) bool {
	for _, prefix := range privateRanges {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}
