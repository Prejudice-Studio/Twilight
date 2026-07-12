package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) syncBangumiForUser(ctx context.Context, uid int64) (synced int, skipped int, failed int, logs []string) {
	u, ok := a.store().User(uid)
	if !ok || !u.BGMMode || u.BGMToken == "" {
		return 0, 0, 0, []string{"用户未开启 Bangumi 同步或未配置个人 Token"}
	}
	records := a.store().PlaybackRecords(uid, 0, 100)
	if len(records) == 0 {
		return 0, 0, 0, []string{"没有待同步的播放记录"}
	}
	syncLogs := a.store().ListBangumiSyncLogs(uid, 1000)
	syncedSet := make(map[string]bool)
	for _, log := range syncLogs {
		if log.Status == "success" && log.RecordItemID != "" {
			syncedSet[log.RecordItemID] = true
		}
	}
	headers := map[string]string{
		"User-Agent":    "Twilight/1.0",
		"Accept":        "application/json",
		"Authorization": "Bearer " + u.BGMToken,
	}
	collectionCache := make(map[string]bool)
	for _, record := range records {
		select {
		case <-ctx.Done():
			logs = append(logs, "同步被中断")
			return synced, skipped, failed, logs
		default:
		}
		if syncedSet[record.ItemID] {
			skipped++
			continue
		}
		subjectID, subjectName, matchErr := a.matchBangumiSubject(ctx, record, headers)
		if matchErr != nil || subjectID == "" {
			failed++
			msg := fmt.Sprintf("匹配失败 [%s]: %v", record.Title, matchErr)
			logs = append(logs, msg)
			a.store().AddBangumiSyncLog(store.BangumiSyncLog{
				UID: uid, RecordItemID: record.ItemID,
				Status: "failed", Message: msg,
			})
			zap.L().Warn("bangumi sync match failed",
				zap.Int64("uid", uid),
				zap.String("item_id", record.ItemID),
				zap.String("title", record.Title),
			)
			continue
		}
		if !collectionCache[subjectID] {
			if err := a.ensureBangumiCollection(ctx, subjectID, headers); err != nil {
				failed++
				msg := fmt.Sprintf("添加收藏失败 [%s]: %v", subjectName, err)
				logs = append(logs, msg)
				a.store().AddBangumiSyncLog(store.BangumiSyncLog{
					UID: uid, RecordItemID: record.ItemID,
					SubjectID: subjectID, SubjectName: subjectName,
					Status: "failed", Message: msg,
				})
				continue
			}
			collectionCache[subjectID] = true
		}
		if record.IndexNumber > 0 {
			if err := a.markBangumiEpisode(ctx, subjectID, record.IndexNumber, u.BGMToken); err != nil {
				failed++
				msg := fmt.Sprintf("标记剧集失败 [%s #%d]: %v", subjectName, record.IndexNumber, err)
				logs = append(logs, msg)
				a.store().AddBangumiSyncLog(store.BangumiSyncLog{
					UID: uid, RecordItemID: record.ItemID,
					SubjectID: subjectID, SubjectName: subjectName,
					Episode: record.IndexNumber,
					Status:  "failed", Message: msg,
				})
				continue
			}
		}
		synced++
		msg := fmt.Sprintf("已同步 [%s]%s", subjectName, episodeSuffix(record.IndexNumber))
		logs = append(logs, msg)
		a.store().AddBangumiSyncLog(store.BangumiSyncLog{
			UID: uid, RecordItemID: record.ItemID,
			SubjectID: subjectID, SubjectName: subjectName,
			Episode: record.IndexNumber,
			Status:  "success", Message: msg,
		})
		syncedSet[record.ItemID] = true
	}
	return synced, skipped, failed, logs
}

func episodeSuffix(ep int) string {
	if ep > 0 {
		return fmt.Sprintf(" 第%d话", ep)
	}
	return ""
}

func (a *App) matchBangumiSubject(ctx context.Context, record store.PlaybackRecord, headers map[string]string) (subjectID, subjectName string, err error) {
	query := record.SeriesName
	if query == "" {
		query = record.Title
	}
	if query == "" {
		return "", "", fmt.Errorf("无法确定条目名称")
	}
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/search/subjects", url.Values{
		"limit":  {"1"},
		"offset": {"0"},
	})
	if err != nil {
		return "", "", err
	}
	body := map[string]any{
		"keyword": query,
		"sort":    "match",
		"filter":  map[string]any{"type": []int{2, 6}, "nsfw": true},
	}
	var payload map[string]any
	if err := postJSON(ctx, endpoint, headers, body, &payload); err != nil {
		return "", "", err
	}
	rows, _ := payload["data"].([]any)
	if len(rows) == 0 {
		return "", "", fmt.Errorf("未找到匹配条目: %s", query)
	}
	item, _ := rows[0].(map[string]any)
	if item == nil {
		return "", "", fmt.Errorf("搜索结果格式异常")
	}
	sid := fmt.Sprint(item["id"])
	sname := firstNonEmpty(asString(item["name_cn"]), asString(item["name"]), sid)
	return sid, sname, nil
}

func (a *App) ensureBangumiCollection(ctx context.Context, subjectID string, headers map[string]string) error {
	endpoint, err := bangumiEndpoint(a.cfg().BangumiAPIURL, "/users/-/collections/"+subjectID, nil)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type": 3,
	}
	var result map[string]any
	if err := postJSON(ctx, endpoint, headers, body, &result); err != nil {
		if strings.Contains(err.Error(), "400") || strings.Contains(err.Error(), "409") {
			return nil
		}
		return err
	}
	return nil
}

func (a *App) markBangumiEpisode(ctx context.Context, subjectID string, episode int, token string) error {
	return a.markBangumiEpisodesThrough(ctx, subjectID, token, episode)
}
