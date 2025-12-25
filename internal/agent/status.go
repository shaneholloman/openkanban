package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/techdufus/openkanban/internal/board"
)

const (
	opencodeDefaultPort = 4096
	opencodeAPITimeout  = 2 * time.Second
)

type opencodeStatusResponse map[string]opencodeSessionStatus

type opencodeSessionStatus struct {
	Type    string `json:"type"`
	Attempt int    `json:"attempt,omitempty"`
	Message string `json:"message,omitempty"`
	Next    int    `json:"next,omitempty"`
}

type StatusDetector struct {
	statusCache     map[string]cachedStatus
	statusCacheMu   sync.RWMutex
	cacheExpiration time.Duration
	statusDirs      []string
	httpClient      *http.Client
}

type cachedStatus struct {
	status    board.AgentStatus
	timestamp time.Time
}

func NewStatusDetector() *StatusDetector {
	homeDir, _ := os.UserHomeDir()

	return &StatusDetector{
		statusCache:     make(map[string]cachedStatus),
		cacheExpiration: 500 * time.Millisecond,
		statusDirs: []string{
			filepath.Join(homeDir, ".cache", "openkanban-status"),
		},
		httpClient: &http.Client{
			Timeout: opencodeAPITimeout,
		},
	}
}

func (d *StatusDetector) DetectStatus(agentType, sessionID string, processRunning bool, terminalContent string) board.AgentStatus {
	return d.DetectStatusWithPath(agentType, sessionID, "", processRunning, terminalContent)
}

func (d *StatusDetector) DetectStatusWithPath(agentType, sessionID, worktreePath string, processRunning bool, terminalContent string) board.AgentStatus {
	if !processRunning {
		return board.AgentNone
	}

	if status := d.readStatusFile(sessionID); status != board.AgentNone {
		return status
	}

	if agentType == "opencode" {
		if worktreePath != "" {
			if status := d.queryOpencodeStatusByDirectory(worktreePath); status != board.AgentNone {
				return status
			}
		}
		if sessionID != "" {
			if status := d.queryOpencodeAPI(sessionID); status != board.AgentNone {
				return status
			}
		}
	}

	if terminalContent != "" {
		if status := d.detectFromTerminalContent(agentType, terminalContent); status != board.AgentNone {
			return status
		}
	}

	return board.AgentIdle
}

func (d *StatusDetector) detectFromTerminalContent(agentType, content string) board.AgentStatus {
	contentLower := strings.ToLower(content)
	lines := strings.Split(content, "\n")

	lastLines := lines
	if len(lines) > 10 {
		lastLines = lines[len(lines)-10:]
	}
	recentContent := strings.Join(lastLines, "\n")
	recentLower := strings.ToLower(recentContent)

	switch agentType {
	case "opencode", "claude":
		return d.detectCodingAgentStatus(recentLower, contentLower)
	default:
		return d.detectGenericAgentStatus(recentLower)
	}
}

func (d *StatusDetector) detectCodingAgentStatus(recentLower, fullLower string) board.AgentStatus {
	waitingPatterns := []string{
		"waiting for",
		"do you want",
		"would you like",
		"[y/n]",
		"(y/n)",
		"press enter",
		"confirm",
		"permission",
		"approve",
	}
	for _, pattern := range waitingPatterns {
		if strings.Contains(recentLower, pattern) {
			return board.AgentWaiting
		}
	}

	workingPatterns := []string{
		"thinking",
		"processing",
		"running",
		"executing",
		"writing",
		"reading",
		"searching",
		"analyzing",
		"generating",
		"...",
		"━",
		"█",
		"▓",
	}
	for _, pattern := range workingPatterns {
		if strings.Contains(recentLower, pattern) {
			return board.AgentWorking
		}
	}

	errorPatterns := []string{
		"error:",
		"failed:",
		"exception:",
		"rate limit",
		"quota exceeded",
		"api error",
	}
	for _, pattern := range errorPatterns {
		if strings.Contains(recentLower, pattern) {
			return board.AgentError
		}
	}

	idlePatterns := []string{
		">",
		"$",
		"ready",
		"idle",
	}
	for _, pattern := range idlePatterns {
		if strings.HasSuffix(strings.TrimSpace(recentLower), pattern) {
			return board.AgentIdle
		}
	}

	return board.AgentNone
}

func (d *StatusDetector) detectGenericAgentStatus(recentLower string) board.AgentStatus {
	if strings.Contains(recentLower, "error") || strings.Contains(recentLower, "failed") {
		return board.AgentError
	}
	if strings.Contains(recentLower, "...") || strings.Contains(recentLower, "processing") {
		return board.AgentWorking
	}
	return board.AgentNone
}

func (d *StatusDetector) queryOpencodeAPI(sessionID string) board.AgentStatus {
	cacheKey := "opencode:" + sessionID

	d.statusCacheMu.RLock()
	cached, exists := d.statusCache[cacheKey]
	d.statusCacheMu.RUnlock()

	if exists && time.Since(cached.timestamp) < d.cacheExpiration {
		return cached.status
	}

	url := fmt.Sprintf("http://localhost:%d/session/status", opencodeDefaultPort)
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return board.AgentNone
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return board.AgentNone
	}

	var statusResp opencodeStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return board.AgentNone
	}

	sessionStatus, found := statusResp[sessionID]
	if !found {
		return board.AgentNone
	}

	status := d.mapOpencodeStatus(sessionStatus)

	d.statusCacheMu.Lock()
	d.statusCache[cacheKey] = cachedStatus{
		status:    status,
		timestamp: time.Now(),
	}
	d.statusCacheMu.Unlock()

	return status
}

func (d *StatusDetector) mapOpencodeStatus(s opencodeSessionStatus) board.AgentStatus {
	switch s.Type {
	case "busy":
		return board.AgentWorking
	case "idle":
		return board.AgentIdle
	case "retry":
		return board.AgentError
	default:
		return board.AgentNone
	}
}

func (d *StatusDetector) queryOpencodeStatusByDirectory(directory string) board.AgentStatus {
	cacheKey := "opencode-dir:" + directory

	d.statusCacheMu.RLock()
	cached, exists := d.statusCache[cacheKey]
	d.statusCacheMu.RUnlock()

	if exists && time.Since(cached.timestamp) < d.cacheExpiration {
		return cached.status
	}

	sessions := d.getOpencodeSessionsForDirectory(directory)
	if len(sessions) == 0 {
		return board.AgentNone
	}

	statusURL := fmt.Sprintf("http://localhost:%d/session/status", opencodeDefaultPort)
	resp, err := d.httpClient.Get(statusURL)
	if err != nil {
		return board.AgentNone
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return board.AgentNone
	}

	var statusResp opencodeStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return board.AgentNone
	}

	var status board.AgentStatus = board.AgentNone
	for _, sessionID := range sessions {
		if sessionStatus, found := statusResp[sessionID]; found {
			mappedStatus := d.mapOpencodeStatus(sessionStatus)
			if mappedStatus == board.AgentWorking {
				status = board.AgentWorking
				break
			}
			if mappedStatus == board.AgentError && status != board.AgentWorking {
				status = board.AgentError
			}
			if mappedStatus == board.AgentIdle && status == board.AgentNone {
				status = board.AgentIdle
			}
		}
	}

	if status != board.AgentNone {
		d.statusCacheMu.Lock()
		d.statusCache[cacheKey] = cachedStatus{
			status:    status,
			timestamp: time.Now(),
		}
		d.statusCacheMu.Unlock()
	}

	return status
}

func (d *StatusDetector) getOpencodeSessionsForDirectory(directory string) []string {
	cmd := exec.Command("opencode", "session", "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var sessions []opencodeSession
	if err := json.Unmarshal(output, &sessions); err != nil {
		return nil
	}

	var matching []string
	for _, s := range sessions {
		if s.Directory == directory {
			matching = append(matching, s.ID)
		}
	}

	return matching
}

func (d *StatusDetector) readStatusFile(sessionName string) board.AgentStatus {
	if sessionName == "" {
		return board.AgentNone
	}

	cacheKey := "file:" + sessionName

	d.statusCacheMu.RLock()
	cached, exists := d.statusCache[cacheKey]
	d.statusCacheMu.RUnlock()

	if exists && time.Since(cached.timestamp) < d.cacheExpiration {
		return cached.status
	}

	var status board.AgentStatus = board.AgentNone

	for _, dir := range d.statusDirs {
		statusFile := filepath.Join(dir, sessionName+".status")
		content, err := os.ReadFile(statusFile)
		if err != nil {
			continue
		}

		statusStr := strings.TrimSpace(string(content))
		switch statusStr {
		case "working":
			status = board.AgentWorking
		case "done", "idle":
			status = board.AgentIdle
		case "waiting", "permission":
			status = board.AgentWaiting
		case "error":
			status = board.AgentError
		case "completed":
			status = board.AgentCompleted
		}

		if status != board.AgentNone {
			break
		}
	}

	d.statusCacheMu.Lock()
	d.statusCache[cacheKey] = cachedStatus{
		status:    status,
		timestamp: time.Now(),
	}
	d.statusCacheMu.Unlock()

	return status
}

func (d *StatusDetector) InvalidateCache(sessionName string) {
	d.statusCacheMu.Lock()
	defer d.statusCacheMu.Unlock()

	if sessionName == "" {
		d.statusCache = make(map[string]cachedStatus)
	} else {
		delete(d.statusCache, "file:"+sessionName)
		delete(d.statusCache, "opencode:"+sessionName)
	}
}

func WriteStatusFile(sessionName string, status board.AgentStatus) error {
	homeDir, _ := os.UserHomeDir()
	statusDir := filepath.Join(homeDir, ".cache", "openkanban-status")

	if err := os.MkdirAll(statusDir, 0755); err != nil {
		return err
	}

	statusFile := filepath.Join(statusDir, sessionName+".status")
	var statusStr string

	switch status {
	case board.AgentWorking:
		statusStr = "working"
	case board.AgentIdle:
		statusStr = "idle"
	case board.AgentWaiting:
		statusStr = "waiting"
	case board.AgentCompleted:
		statusStr = "completed"
	case board.AgentError:
		statusStr = "error"
	default:
		statusStr = "idle"
	}

	return os.WriteFile(statusFile, []byte(statusStr+"\n"), 0644)
}

func CleanupStatusFile(sessionName string) error {
	homeDir, _ := os.UserHomeDir()
	statusDir := filepath.Join(homeDir, ".cache", "openkanban-status")
	statusFile := filepath.Join(statusDir, sessionName+".status")
	os.Remove(statusFile)
	return nil
}
