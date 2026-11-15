package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/analytics"
	"github.com/xpzouying/xiaohongshu-mcp/cookies"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// TopicSnapshotResponse 是一次关键词采集的结果摘要。
type TopicSnapshotResponse struct {
	Keyword      string    `json:"keyword"`
	SnapshotID   int64     `json:"snapshot_id"`
	CollectedAt  time.Time `json:"collected_at"`
	SampledPosts int       `json:"sampled_posts"`
	DatabasePath string    `json:"database_path"`
}

// CollectTopicSnapshot 搜索关键词并保存一份互动数据快照。
func (s *XiaohongshuService) CollectTopicSnapshot(
	ctx context.Context,
	keyword string,
	filters xiaohongshu.FilterOption,
	limit int,
) (*TopicSnapshotResponse, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("关键词不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result, err := s.SearchFeeds(ctx, keyword, filters)
	if err != nil {
		return nil, err
	}
	if len(result.Feeds) > limit {
		result.Feeds = result.Feeds[:limit]
	}

	items := make([]analytics.SnapshotItem, 0, len(result.Feeds))
	for index, feed := range result.Feeds {
		items = append(items, analytics.SnapshotItem{
			FeedID:         feed.ID,
			Rank:           index + 1,
			Title:          feed.NoteCard.DisplayTitle,
			NoteType:       feed.NoteCard.Type,
			AuthorID:       feed.NoteCard.User.UserID,
			AuthorName:     firstNonEmpty(feed.NoteCard.User.Nickname, feed.NoteCard.User.NickName),
			LikedCount:     feed.NoteCard.InteractInfo.LikedCount,
			CollectedCount: feed.NoteCard.InteractInfo.CollectedCount,
			CommentCount:   feed.NoteCard.InteractInfo.CommentCount,
			SharedCount:    feed.NoteCard.InteractInfo.SharedCount,
		})
	}

	collectedAt := time.Now().UTC()
	store, dbPath, err := s.getAnalyticsStore()
	if err != nil {
		return nil, err
	}
	snapshotID, err := store.SaveSnapshot(ctx, analytics.Snapshot{
		Keyword:     keyword,
		CollectedAt: collectedAt,
		Items:       items,
	})
	if err != nil {
		return nil, err
	}

	return &TopicSnapshotResponse{
		Keyword:      keyword,
		SnapshotID:   snapshotID,
		CollectedAt:  collectedAt,
		SampledPosts: len(items),
		DatabasePath: dbPath,
	}, nil
}

func (s *XiaohongshuService) AnalyzeTopics(ctx context.Context, keywords []string) ([]analytics.TopicAnalysis, error) {
	store, _, err := s.getAnalyticsStore()
	if err != nil {
		return nil, err
	}
	return store.AnalyzeTopics(ctx, keywords)
}

func (s *XiaohongshuService) getAnalyticsStore() (*analytics.Store, string, error) {
	s.analyticsMu.Lock()
	defer s.analyticsMu.Unlock()

	if s.analyticsStore != nil || s.analyticsErr != nil {
		return s.analyticsStore, analyticsDatabasePath(), s.analyticsErr
	}

	path := analyticsDatabasePath()
	s.analyticsStore, s.analyticsErr = analytics.Open(path)
	return s.analyticsStore, path, s.analyticsErr
}

func analyticsDatabasePath() string {
	if path := strings.TrimSpace(os.Getenv("XHS_ANALYTICS_DB")); path != "" {
		return path
	}
	cookiePath := cookies.GetCookiesFilePath()
	return filepath.Join(filepath.Dir(cookiePath), "analytics.db")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
