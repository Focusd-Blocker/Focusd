package stats

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

func HandleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

const heartbeatGapThreshold = 15 * time.Minute

type Gap struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type EventLogEntry struct {
	Domain string    `json:"domain"`
	Source string    `json:"source"`
	Count  int       `json:"count,omitempty"`
	At     time.Time `json:"at"`
}

type State struct {
	DaemonVersion   string          `json:"daemon_version"`
	LastEventAt     time.Time       `json:"last_event_at"`
	TotalEvents     int             `json:"total_events"`
	LastHeartbeatAt time.Time       `json:"last_heartbeat_at"`
	Gaps            []Gap           `json:"gaps"`
	RecentEvents    []EventLogEntry `json:"recent_events"`
	DailyStats      map[string]int  `json:"daily_stats"`
	DomainStats     map[string]int  `json:"domain_stats"`
}

const maxRecentEvents = 50

var (
	mu    sync.Mutex
	state State
	path  string
	start = time.Now()
)

func Init(daemonVersion string) error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("resolving cache dir: %w", err)
	}
	path = filepath.Join(cacheDir, "focusd", "stats.json")

	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			state = State{
				DaemonVersion: daemonVersion,
				DailyStats:    make(map[string]int),
				DomainStats:   make(map[string]int),
			}
			saveLocked()
			return nil
		}
		return fmt.Errorf("reading stats file: %w", err)
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parsing stats file: %w", err)
	}

	if state.DailyStats == nil {
		state.DailyStats = make(map[string]int)
	}
	if state.DomainStats == nil {
		state.DomainStats = make(map[string]int)
	}
	if state.DaemonVersion != daemonVersion {
		state.DaemonVersion = daemonVersion
		saveLocked()
	}

	return nil
}

func saveLocked() {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Println("stats: failed to create dir:", err)
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Println("stats: failed to marshal state:", err)
		return
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Println("stats: failed to write state:", err)
	}
}

type EventPayload struct {
	Domain string `json:"domain"`
	Source string `json:"source"`
	Count  int    `json:"count,omitempty"`
}

func HandleEvent(w http.ResponseWriter, r *http.Request) {
	var payload EventPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if payload.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}
	if payload.Count == 0 {
		payload.Count = 1
	}

	now := time.Now().UTC()

	mu.Lock()
	state.LastEventAt = now
	state.TotalEvents += payload.Count

	dateKey := now.Format("2006-01-02")
	state.DailyStats[dateKey] += payload.Count
	state.DomainStats[payload.Domain] += payload.Count

	entry := EventLogEntry{
		Domain: payload.Domain,
		Source: payload.Source,
		Count:  payload.Count,
		At:     now,
	}
	state.RecentEvents = append(state.RecentEvents, entry)
	if len(state.RecentEvents) > maxRecentEvents {
		state.RecentEvents = state.RecentEvents[len(state.RecentEvents)-maxRecentEvents:]
	}

	saveLocked()
	mu.Unlock()

	log.Printf("event: domain=%s source=%s count=%d", payload.Domain, payload.Source, payload.Count)

	w.WriteHeader(http.StatusAccepted)
}

func HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()

	mu.Lock()
	if !state.LastHeartbeatAt.IsZero() {
		gap := now.Sub(state.LastHeartbeatAt)
		if gap > heartbeatGapThreshold {
			state.Gaps = append(state.Gaps, Gap{
				Start: state.LastHeartbeatAt,
				End:   now,
			})
			if len(state.Gaps) > 50 {
				state.Gaps = state.Gaps[len(state.Gaps)-50:]
			}
			log.Printf("heartbeat gap detected: %s to %s (%s)", state.LastHeartbeatAt, now, gap)
		}
	}
	state.LastHeartbeatAt = now
	saveLocked()
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

type StatsResponse struct {
	DaemonVersion    string          `json:"daemon_version"`
	StartedAt        time.Time       `json:"started_at"`
	UptimeSeconds    float64         `json:"uptime_seconds"`
	Goroutines       int             `json:"goroutines"`
	MemoryAllocBytes uint64          `json:"memory_alloc_bytes"`
	MemorySysBytes   uint64          `json:"memory_sys_bytes"`
	OS               string          `json:"os"`
	Architecture     string          `json:"architecture"`
	StreakSince      time.Time       `json:"streak_since"`
	StreakSeconds    float64         `json:"streak_seconds"`
	TotalEvents      int             `json:"total_events"`
	LastHeartbeatAt  time.Time       `json:"last_heartbeat_at"`
	Gaps             []Gap           `json:"gaps"`
	RecentEvents     []EventLogEntry `json:"recent_events"`
	DailyStats       map[string]int  `json:"daily_stats"`
	DomainStats      map[string]int  `json:"domain_stats"`
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	mu.Lock()

	// Create a copy of DailyStats to avoid race conditions during JSON marshalling
	dailyStatsCopy := make(map[string]int, len(state.DailyStats))
	for k, v := range state.DailyStats {
		dailyStatsCopy[k] = v
	}

	domainStatsCopy := make(map[string]int, len(state.DomainStats))
	for k, v := range state.DomainStats {
		domainStatsCopy[k] = v
	}

	resp := StatsResponse{
		DaemonVersion:    state.DaemonVersion,
		StartedAt:        start,
		UptimeSeconds:    time.Since(start).Seconds(),
		Goroutines:       runtime.NumGoroutine(),
		MemoryAllocBytes: memStats.Alloc,
		MemorySysBytes:   memStats.Sys,
		OS:               runtime.GOOS,
		Architecture:     runtime.GOARCH,
		StreakSince:      state.LastEventAt,
		TotalEvents:      state.TotalEvents,
		LastHeartbeatAt:  state.LastHeartbeatAt,
		Gaps:             state.Gaps,
		RecentEvents:     state.RecentEvents,
		DailyStats:       dailyStatsCopy,
		DomainStats:      domainStatsCopy,
	}
	mu.Unlock()

	if !resp.StreakSince.IsZero() {
		resp.StreakSeconds = time.Since(resp.StreakSince).Seconds()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
