package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

// v2ConfigBackupDTO is intentionally smaller than store.BackupInfo. A server
// filesystem path is an implementation detail and must not cross the API
// boundary, even for an administrator.
func v2ConfigBackupDTO(info store.BackupInfo) map[string]any {
	item := map[string]any{
		"name":       info.Name,
		"size":       info.Size,
		"created_at": info.CreatedAt,
	}
	if info.Note != "" {
		item["note"] = info.Note
	}
	return item
}

func (a *App) handleV2ConfigTOMLGet(w http.ResponseWriter, r *http.Request, _ Params) {
	data, err := os.ReadFile(a.configFilePath())
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrConfigFileNotFound, "配置文件不存在")
		return
	}
	maskedValues := configValues(*a.cfg())
	maskConfigSecrets(maskedValues)
	normalizedContent := stripProtectedAdminConfig(renderConfigTOML(maskedValues))
	rawContent := stripProtectedAdminConfig(maskTOMLSecrets(string(data)))
	ok(w, "OK", map[string]any{
		"content":     normalizedContent,
		"raw_content": rawContent,
		"completed":   normalizedContent != rawContent,
	})
}

func (a *App) handleV2ConfigBackups(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, err := listConfigBackups(a.configBackupDir())
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrConfigBackupListFailed, "读取配置备份列表失败")
		return
	}
	items := make([]map[string]any, 0, len(backups))
	for _, backup := range backups {
		items = append(items, v2ConfigBackupDTO(backup))
	}
	ok(w, "OK", map[string]any{"backups": items})
}

func (a *App) handleV2ConfigBackupInspect(w http.ResponseWriter, r *http.Request, params Params) {
	backup, content, err := a.configBackupContent(params["name"])
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupInvalid, "配置备份无效")
		return
	}
	ok(w, "OK", map[string]any{
		"backup":  v2ConfigBackupDTO(backup),
		"content": stripProtectedAdminConfig(maskTOMLSecrets(string(content))),
	})
}

// v2BufferedResponseWriter lets compatibility handlers keep one implementation
// of validation, persistence and audit behavior while removing implementation
// details from their V2 success payloads.
type v2BufferedResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *v2BufferedResponseWriter) Header() http.Header { return w.header }

func (w *v2BufferedResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *v2BufferedResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func redactV2PrivateFields(value any, blocked map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for key := range typed {
			if _, blocked := blocked[strings.ToLower(key)]; blocked {
				delete(typed, key)
				continue
			}
			redactV2PrivateFields(typed[key], blocked)
		}
	case []any:
		for _, item := range typed {
			redactV2PrivateFields(item, blocked)
		}
	}
}

func sanitizeV2Envelope(body []byte, blocked map[string]struct{}) []byte {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	redactV2PrivateFields(envelope["data"], blocked)
	result, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return append(result, '\n')
}

func (a *App) delegateV2SafeHandler(w http.ResponseWriter, r *http.Request, p Params, handler func(http.ResponseWriter, *http.Request, Params), blocked map[string]struct{}) {
	buffered := &v2BufferedResponseWriter{header: make(http.Header)}
	handler(buffered, r, p)
	for key, values := range buffered.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	status := buffered.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(sanitizeV2Envelope(buffered.body.Bytes(), blocked))
}

var v2ConfigPrivateFields = map[string]struct{}{
	"path": {}, "config_file": {}, "backup_dir": {}, "backup_path": {},
}

func (a *App) delegateV2ConfigHandler(w http.ResponseWriter, r *http.Request, p Params, handler func(http.ResponseWriter, *http.Request, Params)) {
	a.delegateV2SafeHandler(w, r, p, handler, v2ConfigPrivateFields)
}

func (a *App) handleV2ConfigSchema(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigSchemaFull)
}

func (a *App) handleV2ConfigSchemaUpdate(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigSchemaUpdateSafe)
}

func (a *App) handleV2ConfigTOMLUpdate(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigTOMLPutSafe)
}

func (a *App) handleV2ConfigBackup(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigBackup)
}

func (a *App) handleV2ConfigBackupDelete(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigBackupDelete)
}

func (a *App) handleV2ConfigRestore(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigRestore)
}

func (a *App) handleV2ConfigSweep(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2ConfigHandler(w, r, p, a.handleConfigSweep)
}

func (a *App) handleV2ConfigBackgroundUpload(w http.ResponseWriter, r *http.Request, p Params) {
	// The upload handler already returns a logical asset URL and no filesystem
	// path. Keeping it behind the same adapter still gives V2 its own resource.
	a.handleUploadAuthBackground(w, r, p)
}
