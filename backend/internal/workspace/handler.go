package workspace

import (
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"

	sacv1 "g.echo.tech/dev/sac/gen/sac/v1"
	"g.echo.tech/dev/sac/internal/auth"
	"g.echo.tech/dev/sac/internal/storage"
	"g.echo.tech/dev/sac/pkg/protobind"
	"g.echo.tech/dev/sac/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxUploadSize = 100 << 20 // 100MB

// contentTypeByFilename returns a MIME type based on file extension, falling back to octet-stream.
func contentTypeByFilename(fileName string) string {
	ext := path.Ext(fileName)
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// Handler serves workspace HTTP endpoints (output download, internal upload, shared files).
type Handler struct {
	db       *bun.DB
	provider *storage.StorageProvider
	hub      *OutputHub
	jwt      *auth.JWTService
}

// NewHandler creates a new workspace handler.
func NewHandler(db *bun.DB, provider *storage.StorageProvider, hub *OutputHub, jwt *auth.JWTService) *Handler {
	return &Handler{db: db, provider: provider, hub: hub, jwt: jwt}
}

// requireOSS is a middleware that checks if storage is configured.
func (h *Handler) requireOSS() gin.HandlerFunc {
	return func(c *gin.Context) {
		backend := h.provider.GetClient(c.Request.Context())
		if backend == nil {
			response.ServiceUnavailable(c, "Workspace storage is not configured. Ask your admin to configure storage settings.")
			c.Abort()
			return
		}
		c.Set("storageBackend", backend)
		c.Next()
	}
}

// RequireOSS is the exported version for use in main.go route registration.
func (h *Handler) RequireOSS() gin.HandlerFunc {
	return h.requireOSS()
}

// getOSS retrieves the StorageBackend stored by requireOSS middleware.
func (h *Handler) getOSS(c *gin.Context) storage.StorageBackend {
	v, _ := c.Get("storageBackend")
	return v.(storage.StorageBackend)
}

// GetStatus returns whether OSS is configured.
func (h *Handler) GetStatus(c *gin.Context) {
	configured := h.provider.IsConfigured(c.Request.Context())
	protobind.OK(c, &sacv1.WorkspaceStatusResponse{Configured: configured})
}

// parseAgentID extracts and validates the agent_id parameter from query or form.
func parseAgentID(c *gin.Context) (int64, bool) {
	agentIDStr := c.Query("agent_id")
	if agentIDStr == "" {
		agentIDStr = c.PostForm("agent_id")
	}
	if agentIDStr == "" {
		response.BadRequest(c, "agent_id parameter required")
		return 0, false
	}
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil || agentID <= 0 {
		response.BadRequest(c, "invalid agent_id")
		return 0, false
	}
	return agentID, true
}

// storageItemsToProto converts storage list items to proto FileItem messages.
func storageItemsToProto(items []storage.ObjectInfo, basePrefix string) []*sacv1.FileItem {
	var files []*sacv1.FileItem
	for _, item := range items {
		relPath := strings.TrimPrefix(item.Key, basePrefix)
		name := path.Base(relPath)
		if item.IsDirectory {
			name = path.Base(strings.TrimSuffix(relPath, "/"))
			if name == "." || name == "" {
				continue
			}
		}
		fi := &sacv1.FileItem{
			Name:        name,
			Path:        relPath,
			Size:        item.Size,
			IsDirectory: item.IsDirectory,
		}
		if !item.LastModified.IsZero() {
			fi.LastModified = timestamppb.New(item.LastModified)
		}
		files = append(files, fi)
	}
	return files
}

// sanitizePath cleans a file path, preventing directory traversal.
func sanitizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimLeft(p, "/")
	p = strings.ReplaceAll(p, "..", "")
	p = strings.ReplaceAll(p, "//", "/")
	return p
}

// ossKeyPrefix returns the OSS key prefix for a user's agent workspace.
func ossKeyPrefix(userID, agentID int64) string {
	return fmt.Sprintf("users/%d/agents/%d/", userID, agentID)
}
