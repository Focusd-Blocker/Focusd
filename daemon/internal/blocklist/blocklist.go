package blocklist

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func fetchBlockedDomains() ([]string, error) {
	urls := []string{
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/porn-only/hosts",
		"https://raw.githubusercontent.com/blocklistproject/Lists/master/porn.txt",
	}

	uniqueDomains := make(map[string]struct{})

	for _, u := range urls {
		resp, err := httpClient.Get(u)
		if err != nil {
			log.Printf("failed to fetch %s: %v", u, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("unexpected status for %s: %s", u, resp.Status)
			resp.Body.Close()
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) > 1 && (fields[0] == "0.0.0.0" || fields[0] == "127.0.0.1") {
				uniqueDomains[fields[1]] = struct{}{}
			} else if len(fields) == 1 {
				uniqueDomains[fields[0]] = struct{}{}
			}
		}
		resp.Body.Close()
	}

	var domains []string
	for d := range uniqueDomains {
		domains = append(domains, d)
	}

	if len(domains) == 0 {
		return nil, fmt.Errorf("no domains fetched from any source")
	}

	return domains, nil
}

func hostsFilePath() string {
	cacheDir, _ := os.UserCacheDir()
	return filepath.Join(cacheDir, "focusd", "hosts.txt")
}

func hostsFilePathOld() string {
	cacheDir, _ := os.UserCacheDir()
	return filepath.Join(cacheDir, "focusd", "hosts.json")
}

func saveHostsList(path string, newDomains []string) error {
	existing, err := loadDomainsFromDisk(path)
	if err != nil {
		return err
	}

	if existing != nil {
		existingSet := make(map[string]struct{}, len(existing))
		for _, d := range existing {
			existingSet[d] = struct{}{}
		}
		if len(existingSet) == len(newDomains) {
			same := true
			for _, d := range newDomains {
				if _, ok := existingSet[d]; !ok {
					same = false
					break
				}
			}
			if same {
				log.Println("no changes, skipping write")
				return nil
			}
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	w := bufio.NewWriter(f)
	for _, d := range newDomains {
		w.WriteString(d)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing domains: %w", err)
	}
	defer f.Close()

	log.Println("updated", path)
	return nil
}

func loadDomainsFromDisk(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			domains = append(domains, line)
		}
	}
	return domains, scanner.Err()
}

func migrateOldFormatIfNeeded() {
	old := hostsFilePathOld()
	new := hostsFilePath()

	if _, err := os.Stat(new); err == nil {
		os.Remove(old)
		return
	}

	data, err := os.ReadFile(old)
	if err != nil {
		return
	}

	var list struct {
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal(data, &list); err != nil || len(list.Domains) == 0 {
		return
	}

	if err := saveHostsList(new, list.Domains); err == nil {
		os.Remove(old)
		log.Println("migrated hosts.json to hosts.txt")
	}
}

func updateLists() error {
	migrateOldFormatIfNeeded()

	newestDomains, err := fetchBlockedDomains()
	if err != nil {
		return fmt.Errorf("[updateLists] unable to fetch domains: %w", err)
	}

	if err := saveHostsList(hostsFilePath(), newestDomains); err != nil {
		log.Println("error:", err)
	}

	return nil
}

func StartUpdateLoop(ctx context.Context) {
	if err := updateLists(); err != nil {
		log.Println("initial update failed:", err)
	}

	ticker := time.NewTicker(6 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := updateLists(); err != nil {
					log.Println("scheduled update failed:", err)
				}
			}
		}
	}()
}

func cleanDomain(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return ""
	}
	if !strings.Contains(input, "://") && (strings.HasPrefix(input, "http/") || strings.HasPrefix(input, "https/")) {
		input = strings.Replace(input, "http/", "http://", 1)
		input = strings.Replace(input, "https/", "https://", 1)
	}

	var host string
	if !strings.Contains(input, "://") {
		tempURL := "http://" + input
		u, err := url.Parse(tempURL)
		if err == nil {
			host = u.Hostname()
		}
	} else {
		u, err := url.Parse(input)
		if err == nil {
			host = u.Hostname()
		}
	}

	if host == "" {
		host = input
		if idx := strings.Index(host, "://"); idx != -1 {
			host = host[idx+3:]
		}
		if idx := strings.Index(host, "/"); idx != -1 {
			host = host[:idx]
		}
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
	}

	host = strings.TrimPrefix(host, "www.")
	return strings.TrimSpace(host)
}

func getCustomBlockedList() ([]string, error) {
	cacheDir, _ := os.UserCacheDir()
	path := filepath.Join(cacheDir, "focusd", "custom_hosts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var domains []string
	if err := json.Unmarshal(data, &domains); err != nil {
		return nil, err
	}

	cleaned := make([]string, 0, len(domains))
	seen := make(map[string]bool)
	dirty := false
	for _, d := range domains {
		c := cleanDomain(d)
		if c != "" && !seen[c] {
			seen[c] = true
			cleaned = append(cleaned, c)
		}
		if c != d {
			dirty = true
		}
	}
	if dirty || len(cleaned) < len(domains) {
		saveCustomBlockedList(cleaned)
	}

	return cleaned, nil
}

func saveCustomBlockedList(domains []string) error {
	cacheDir, _ := os.UserCacheDir()
	path := filepath.Join(cacheDir, "focusd", "custom_hosts.json")
	data, err := json.MarshalIndent(domains, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating custom blocklist dir: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func HandleBlocklist(w http.ResponseWriter, r *http.Request) {
	domains, err := loadDomainsFromDisk(hostsFilePath())
	if err != nil {
		http.Error(w, "failed to load blocklist", http.StatusInternalServerError)
		return
	}
	if domains == nil {
		http.Error(w, "blocklist not loaded yet", http.StatusServiceUnavailable)
		return
	}

	custom, err := getCustomBlockedList()
	if err != nil {
		http.Error(w, "failed to load custom blocklist", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"domains":[`)

	first := true
	for _, d := range domains {
		if !first {
			w.Write([]byte(","))
		}
		first = false
		b, _ := json.Marshal(d)
		w.Write(b)
	}
	for _, d := range custom {
		if !first {
			w.Write([]byte(","))
		}
		first = false
		b, _ := json.Marshal(d)
		w.Write(b)
	}

	w.Write([]byte(`]}`))
}

var (
	clientsMu sync.Mutex
	clients   = make(map[chan struct{}]struct{})
)

func BroadcastUpdate() {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for ch := range clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func HandleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, ": keepalive\n\n")
	flusher.Flush()

	ch := make(chan struct{}, 1)
	clientsMu.Lock()
	clients[ch] = struct{}{}
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, ch)
		clientsMu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			fmt.Fprintf(w, "event: update\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

func HandleCustomBlocklist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		custom, err := getCustomBlockedList()
		if err != nil {
			http.Error(w, "failed to load custom blocklist", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(custom)
		return
	}

	if r.Method == http.MethodPost {
		var payload struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		domain := cleanDomain(payload.Domain)
		if domain == "" {
			http.Error(w, "invalid domain", http.StatusBadRequest)
			return
		}
		custom, err := getCustomBlockedList()
		if err != nil {
			http.Error(w, "failed to load custom blocklist", http.StatusInternalServerError)
			return
		}
		for _, d := range custom {
			if d == domain {
				json.NewEncoder(w).Encode(custom)
				return
			}
		}
		custom = append(custom, domain)
		if err := saveCustomBlockedList(custom); err != nil {
			http.Error(w, "failed to save custom blocklist", http.StatusInternalServerError)
			return
		}
		BroadcastUpdate()
		json.NewEncoder(w).Encode(custom)
		return
	}

	if r.Method == http.MethodDelete {
		var payload struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		domain := cleanDomain(payload.Domain)
		if domain == "" {
			http.Error(w, "invalid domain", http.StatusBadRequest)
			return
		}
		custom, err := getCustomBlockedList()
		if err != nil {
			http.Error(w, "failed to load custom blocklist", http.StatusInternalServerError)
			return
		}
		var updated []string
		for _, d := range custom {
			if d != domain {
				updated = append(updated, d)
			}
		}
		if err := saveCustomBlockedList(updated); err != nil {
			http.Error(w, "failed to save custom blocklist", http.StatusInternalServerError)
			return
		}
		BroadcastUpdate()
		json.NewEncoder(w).Encode(updated)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
