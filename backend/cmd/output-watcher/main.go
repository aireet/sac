package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"g.echo.tech/dev/sac/pkg/logger"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

const debounceTime = 500 * time.Millisecond

var watchDir = "/workspace/output"

func main() {
	logger.Init(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))

	userID := os.Getenv("USER_ID")
	agentID := os.Getenv("AGENT_ID")
	apiURL := os.Getenv("SAC_API_URL")

	if userID == "" || agentID == "" || apiURL == "" {
		log.Fatal().Msg("USER_ID, AGENT_ID, and SAC_API_URL environment variables are required")
	}

	if dir := os.Getenv("WATCH_DIR"); dir != "" {
		watchDir = dir
	}

	// Ensure watch directory exists
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		log.Fatal().Err(err).Str("dir", watchDir).Msg("failed to create watch directory")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create watcher")
	}
	defer watcher.Close()

	// Add the root watch directory
	if err := watcher.Add(watchDir); err != nil {
		log.Fatal().Err(err).Str("dir", watchDir).Msg("failed to watch directory")
	}

	// Recursively add existing subdirectories
	filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && path != watchDir {
			watcher.Add(path)
		}
		return nil
	})

	// Upload existing files on startup
	go uploadExistingFiles(apiURL, userID, agentID)

	log.Info().Str("dir", watchDir).Str("user_id", userID).Str("agent_id", agentID).Msg("output-watcher started")

	// Debounce map: filepath -> timer
	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Ignore hidden files and temp files
			base := filepath.Base(event.Name)
			if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
				continue
			}

			if event.Has(fsnotify.Create) {
				// Check if it's a new directory — start watching it
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					watcher.Add(event.Name)
					log.Debug().Str("dir", event.Name).Msg("watching new directory")
					continue
				}
			}

			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
				// Debounce: reset timer for this file
				mu.Lock()
				if t, exists := timers[event.Name]; exists {
					t.Stop()
				}
				timers[event.Name] = time.AfterFunc(debounceTime, func() {
					mu.Lock()
					delete(timers, event.Name)
					mu.Unlock()
					uploadFile(apiURL, userID, agentID, event.Name)
				})
				mu.Unlock()
			}

			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				// Cancel any pending upload for this file
				mu.Lock()
				if t, exists := timers[event.Name]; exists {
					t.Stop()
					delete(timers, event.Name)
				}
				mu.Unlock()
				deleteFile(apiURL, userID, agentID, event.Name)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("watcher error")
		}
	}
}

// relPath returns the path relative to watchDir.
func relPath(absPath string) string {
	rel, err := filepath.Rel(watchDir, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return rel
}

// uploadExistingFiles uploads all files already in the watch directory on startup.
func uploadExistingFiles(apiURL, userID, agentID string) {
	filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") {
			return nil
		}
		uploadFile(apiURL, userID, agentID, path)
		return nil
	})
}

// uploadFile sends a file to the internal API as multipart form data.
func uploadFile(apiURL, userID, agentID, filePath string) {
	rel := relPath(filePath)

	f, err := os.Open(filePath)
	if err != nil {
		log.Warn().Err(err).Str("path", filePath).Msg("failed to open file")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("user_id", userID)
	writer.WriteField("agent_id", agentID)
	writer.WriteField("path", rel)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		log.Warn().Err(err).Str("path", rel).Msg("failed to create form file")
		return
	}
	if _, err := io.Copy(part, f); err != nil {
		log.Warn().Err(err).Str("path", rel).Msg("failed to copy file")
		return
	}
	writer.Close()

	url := fmt.Sprintf("%s/api/internal/output/upload", apiURL)
	resp, err := http.Post(url, writer.FormDataContentType(), &buf)
	if err != nil {
		log.Warn().Err(err).Str("path", rel).Msg("failed to upload file")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Warn().Str("path", rel).Int("status", resp.StatusCode).Str("body", string(body)).Msg("upload failed")
		return
	}

	log.Debug().Str("path", rel).Msg("uploaded")
}

// deleteFile sends a delete notification to the internal API.
func deleteFile(apiURL, userID, agentID, filePath string) {
	rel := relPath(filePath)

	payload := map[string]interface{}{
		"user_id":  mustParseInt(userID),
		"agent_id": mustParseInt(agentID),
		"path":     rel,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/api/internal/output/delete", apiURL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Warn().Err(err).Str("path", rel).Msg("failed to delete file")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Warn().Str("path", rel).Int("status", resp.StatusCode).Str("body", string(respBody)).Msg("delete failed")
		return
	}

	log.Debug().Str("path", rel).Msg("deleted")
}

func mustParseInt(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
