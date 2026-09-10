package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
)

func (a *App) handleV2ExportUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "email", "role", "active", "emby_id", "telegram_id", "expired_at"})

	for _, u := range a.store().ListUsers() {
		embyID := ""
		if u.EmbyID != "" {
			embyID = u.EmbyID
		}
		telegramID := ""
		if u.TelegramID != 0 {
			telegramID = strconv.FormatInt(u.TelegramID, 10)
		}
		expiredAt := ""
		if u.ExpiredAt > 0 {
			expiredAt = strconv.FormatInt(u.ExpiredAt, 10)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(u.UID, 10),
			csvSafe(u.Username),
			csvSafe(u.Email),
			strconv.Itoa(u.Role),
			strconv.FormatBool(u.Active),
			csvSafe(embyID),
			csvSafe(telegramID),
			expiredAt,
		})
	}
	cw.Flush()
}
