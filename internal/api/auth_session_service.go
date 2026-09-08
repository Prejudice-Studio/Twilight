package api

import (
	"context"
	"time"
)

// refreshSession rotates one authenticated session. The HTTP layer owns the
// response envelope and cookie headers; this function owns the state change so
// V1 compatibility routes and V2 SSR resources cannot drift apart.
func (a *App) refreshSession(ctx context.Context, token string, uid int64) (string, time.Time, error) {
	a.sessions().Delete(ctx, token)
	return a.sessions().Create(ctx, uid)
}

func (a *App) revokeSession(ctx context.Context, token string) {
	a.sessions().Delete(ctx, token)
}

func (a *App) revokeAllSessions(ctx context.Context, uid int64) {
	a.sessions().DeleteUser(ctx, uid)
}
