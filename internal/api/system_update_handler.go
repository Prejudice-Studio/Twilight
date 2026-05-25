package api

import (
	"net/http"
)

func (a *App) handleSystemUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	repoURL := firstNonEmpty(stringValue(payload, "repo_url"), a.cfg().SystemUpdateRepoURL)
	branch := firstNonEmpty(stringValue(payload, "branch"), a.cfg().SystemUpdateBranch, "main")
	restart := boolValue(payload, "restart_services", a.cfg().SystemUpdateRestartServices)
	dryRun := boolValue(payload, "dry_run", false)
	allowDirty := boolValue(payload, "allow_dirty", false)
	result := applyGitUpdate(r.Context(), repoURL, branch, restart, dryRun, allowDirty)
	if !boolish(result["success"]) {
		// 走标准 envelope 而非裸 map：保留 result 作为 data 字段下发，
		// error_code 由 result["error_code"] 透出，前端基于稳定码做分支
		status := int(numeric(result["code"]))
		if status < 400 {
			status = http.StatusInternalServerError
		}
		errCode, _ := result["error_code"].(ErrCode)
		if errCode == "" {
			errCode = ErrInternal
		}
		failWithCodeData(w, status, errCode, asString(result["message"]), result)
		return
	}
	ok(w, asString(result["message"]), result)
}
