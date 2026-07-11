# Playback Stats

Twilight playback stats are based on Emby ActivityLog entries and are independent from the Bangumi webhook playback-record path. The collector fetches playback events from Emby `/System/ActivityLog/Entries`, stores them in `store.EmbyActivityLogs`, and aggregates them by scope and time window.

## Feature Gate And Collection

- Config key: `[Emby] emby_playback_stats_enabled = true`.
- Collector: `fetchAndStoreEmbyActivityLogs`, which fetches up to 500 recent Emby activity entries with user information per request.
- Storage limit: `maxEmbyActivityLogs = 20000`; entries are deduplicated by Emby log ID and oldest entries are pruned when the cap is exceeded.
- Auto refresh: `GET /admin/emby/activity-logs` and playback stats endpoints trigger a lightweight 2-minute throttled refresh by default. `?refresh=1` forces an immediate fetch.
- Scheduler: `sync_emby_activity_logs` can still be configured for proactive background sync in production.

## Endpoints

| Path | Auth | Description |
| ---- | ---- | ---- |
| `GET /api/v1/emby/playback-stats` | User | Current-user stats |
| `GET /api/v1/admin/emby/playback-stats` | Admin | Global, self, or selected-user stats |
| `GET /api/v1/admin/emby/playback-stats/:uid` | Admin | Stats for a specific local UID |
| `GET /api/v1/admin/emby/activity-logs` | Admin | Raw Emby activity logs |

Common query parameters:

- `scope=self|global|user`: stats scope. Non-admin users can only view `self`.
- `uid=<uid>`: local UID used when `scope=user`.
- `days=7|30|90|365`: rolling window.
- `today=1`: current-day window; overrides `days`.
- `limit=10|20|50|100`: ranking size.
- `sort=plays|name`: top-item sort mode.
- `refresh=1`: force refresh Emby activity logs before aggregation.

## Frontend Behavior

- `/stats/playback` auto-loads stats and polls only while the page is visible, using the selected 30s, 60s, or 5m interval.
- The manual refresh button force-fetches Emby activity logs before recomputing stats.
- The `/admin/emby` activity-log panel polls every 30s by default and exposes an off switch.
- The `/admin/emby` device/IP audit panel polls online status every 30s by default; manual refresh sends `refresh=1`.

## Limitations

Emby ActivityLog reliably provides playback event counts, user, item name, and timestamp. It does not reliably provide total playback duration, so `total_duration` is currently a reserved field and returns `0`. Precise duration stats should use session stop events or another Emby playback-reporting data source in a future change.
