package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Snapshot 是一次关键词搜索结果快照。
type Snapshot struct {
	Keyword     string
	CollectedAt time.Time
	Items       []SnapshotItem
}

// SnapshotItem 是快照中的一条笔记，只保存分析所需字段，不保存 xsec_token。
type SnapshotItem struct {
	FeedID         string
	Rank           int
	Title          string
	NoteType       string
	AuthorID       string
	AuthorName     string
	LikedCount     string
	CollectedCount string
	CommentCount   string
	SharedCount    string
}

// TopicAnalysis 是一个关键词的历史分析结果。
type TopicAnalysis struct {
	Keyword           string    `json:"keyword"`
	HasSnapshot       bool      `json:"has_snapshot"`
	LatestAt          time.Time `json:"latest_at,omitempty"`
	HeatScore         float64   `json:"heat_score"`
	GrowthRate        float64   `json:"growth_rate"`
	HasGrowthBaseline bool      `json:"has_growth_baseline"`
	SampledPosts      int       `json:"sampled_posts"`
	UniqueAuthors     int       `json:"unique_authors"`
	TopTitles         []string  `json:"top_titles,omitempty"`
}

// Store 持有话题快照 SQLite 数据库。
type Store struct {
	db *sql.DB
}

// Open 打开或创建 SQLite 数据库。
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("analytics database path is empty")
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create analytics database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open analytics database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) init() error {
	statements := []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`CREATE TABLE IF NOT EXISTS topic_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT NOT NULL,
			collected_at TEXT NOT NULL,
			sampled_posts INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_snapshots_keyword_time
			ON topic_snapshots(keyword, collected_at DESC)`,
		`CREATE TABLE IF NOT EXISTS topic_snapshot_items (
			snapshot_id INTEGER NOT NULL,
			feed_id TEXT NOT NULL,
			rank INTEGER NOT NULL DEFAULT 0,
			title TEXT NOT NULL DEFAULT '',
			note_type TEXT NOT NULL DEFAULT '',
			author_id TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL DEFAULT '',
			liked_count INTEGER NOT NULL DEFAULT 0,
			collected_count INTEGER NOT NULL DEFAULT 0,
			comment_count INTEGER NOT NULL DEFAULT 0,
			shared_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(snapshot_id, feed_id),
			FOREIGN KEY(snapshot_id) REFERENCES topic_snapshots(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_topic_snapshot_items_snapshot
			ON topic_snapshot_items(snapshot_id)`,
	}

	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize analytics database: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveSnapshot 将一次搜索结果以事务形式写入数据库。
func (s *Store) SaveSnapshot(ctx context.Context, snapshot Snapshot) (int64, error) {
	keyword := normalizeKeyword(snapshot.Keyword)
	if keyword == "" {
		return 0, fmt.Errorf("snapshot keyword is empty")
	}
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin snapshot transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO topic_snapshots(keyword, collected_at, sampled_posts) VALUES (?, ?, ?)`,
		keyword, snapshot.CollectedAt.UTC().Format(time.RFC3339Nano), len(snapshot.Items))
	if err != nil {
		return 0, fmt.Errorf("insert topic snapshot: %w", err)
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read topic snapshot id: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO topic_snapshot_items(
			snapshot_id, feed_id, rank, title, note_type, author_id, author_name,
			liked_count, collected_count, comment_count, shared_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare topic snapshot item: %w", err)
	}
	defer stmt.Close()

	for index, item := range snapshot.Items {
		feedID := strings.TrimSpace(item.FeedID)
		if feedID == "" {
			continue
		}
		rank := item.Rank
		if rank <= 0 {
			rank = index + 1
		}
		if _, err := stmt.ExecContext(ctx,
			snapshotID,
			feedID,
			rank,
			item.Title,
			item.NoteType,
			item.AuthorID,
			item.AuthorName,
			parseCountOrZero(item.LikedCount),
			parseCountOrZero(item.CollectedCount),
			parseCountOrZero(item.CommentCount),
			parseCountOrZero(item.SharedCount),
		); err != nil {
			return 0, fmt.Errorf("insert topic snapshot item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit topic snapshot: %w", err)
	}
	return snapshotID, nil
}

// AnalyzeTopics 分析每个关键词最近两次快照。
func (s *Store) AnalyzeTopics(ctx context.Context, keywords []string) ([]TopicAnalysis, error) {
	results := make([]TopicAnalysis, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, rawKeyword := range keywords {
		keyword := normalizeKeyword(rawKeyword)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}

		analysis, err := s.analyzeTopic(ctx, keyword)
		if err != nil {
			return nil, err
		}
		results = append(results, analysis)
	}
	return results, nil
}

type snapshotMeta struct {
	id           int64
	collectedAt  time.Time
	sampledPosts int
}

type storedItem struct {
	title          string
	authorID       string
	likedCount     int64
	collectedCount int64
	commentCount   int64
	sharedCount    int64
}

func (s *Store) analyzeTopic(ctx context.Context, keyword string) (TopicAnalysis, error) {
	analysis := TopicAnalysis{Keyword: keyword}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, collected_at, sampled_posts
		FROM topic_snapshots
		WHERE keyword = ?
		ORDER BY collected_at DESC, id DESC
		LIMIT 2
	`, keyword)
	if err != nil {
		return analysis, fmt.Errorf("query topic snapshots: %w", err)
	}
	defer rows.Close()

	metas := make([]snapshotMeta, 0, 2)
	for rows.Next() {
		var meta snapshotMeta
		var collectedAt string
		if err := rows.Scan(&meta.id, &collectedAt, &meta.sampledPosts); err != nil {
			return analysis, fmt.Errorf("scan topic snapshot: %w", err)
		}
		meta.collectedAt, err = time.Parse(time.RFC3339Nano, collectedAt)
		if err != nil {
			return analysis, fmt.Errorf("parse topic snapshot time: %w", err)
		}
		metas = append(metas, meta)
	}
	if err := rows.Err(); err != nil {
		return analysis, fmt.Errorf("iterate topic snapshots: %w", err)
	}
	if len(metas) == 0 {
		return analysis, nil
	}

	latestItems, err := s.loadItems(ctx, metas[0].id)
	if err != nil {
		return analysis, err
	}
	latestHeat, authors, titles := summarizeItems(latestItems)
	analysis.HasSnapshot = true
	analysis.LatestAt = metas[0].collectedAt
	analysis.HeatScore = latestHeat
	analysis.SampledPosts = metas[0].sampledPosts
	analysis.UniqueAuthors = authors
	analysis.TopTitles = titles

	if len(metas) > 1 {
		previousItems, err := s.loadItems(ctx, metas[1].id)
		if err != nil {
			return analysis, err
		}
		previousHeat, _, _ := summarizeItems(previousItems)
		analysis.HasGrowthBaseline = true
		if previousHeat > 0 {
			analysis.GrowthRate = (latestHeat - previousHeat) / previousHeat * 100
		}
	}
	return analysis, nil
}

func (s *Store) loadItems(ctx context.Context, snapshotID int64) ([]storedItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT title, author_id, liked_count, collected_count, comment_count, shared_count
		FROM topic_snapshot_items
		WHERE snapshot_id = ?
	`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("query topic snapshot items: %w", err)
	}
	defer rows.Close()

	items := make([]storedItem, 0)
	for rows.Next() {
		var item storedItem
		if err := rows.Scan(&item.title, &item.authorID, &item.likedCount, &item.collectedCount, &item.commentCount, &item.sharedCount); err != nil {
			return nil, fmt.Errorf("scan topic snapshot item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topic snapshot items: %w", err)
	}
	return items, nil
}

func summarizeItems(items []storedItem) (float64, int, []string) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	authors := make(map[string]struct{})
	type scoredTitle struct {
		title string
		score float64
	}
	scored := make([]scoredTitle, 0, len(items))
	var total float64
	for _, item := range items {
		if item.authorID != "" {
			authors[item.authorID] = struct{}{}
		}
		score := engagementScore(item)
		total += score
		if item.title != "" {
			scored = append(scored, scoredTitle{title: item.title, score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > 5 {
		scored = scored[:5]
	}
	titles := make([]string, 0, len(scored))
	for _, item := range scored {
		titles = append(titles, item.title)
	}
	return total / float64(len(items)), len(authors), titles
}

// engagementScore 是第一版的透明代理：点赞 + 收藏×2 + 评论×3 + 分享×2。
func engagementScore(item storedItem) float64 {
	return float64(item.likedCount + item.collectedCount*2 + item.commentCount*3 + item.sharedCount*2)
}

// ParseCount 解析小红书常见的互动数字格式。
func ParseCount(raw string) (int64, error) {
	value := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(raw, ",", "")))
	if value == "" {
		return 0, nil
	}
	multiplier := float64(1)
	for _, suffix := range []struct {
		text  string
		value float64
	}{
		{text: "亿", value: 100000000},
		{text: "万", value: 10000},
		{text: "w", value: 10000},
		{text: "千", value: 1000},
		{text: "k", value: 1000},
	} {
		if strings.HasSuffix(value, suffix.text) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix.text))
			multiplier = suffix.value
			break
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse count %q: %w", raw, err)
	}
	return int64(math.Round(number * multiplier)), nil
}

func parseCountOrZero(raw string) int64 {
	value, err := ParseCount(raw)
	if err != nil {
		return 0
	}
	return value
}

func normalizeKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}
