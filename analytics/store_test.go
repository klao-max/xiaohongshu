package analytics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCount(t *testing.T) {
	tests := []struct {
		raw  string
		want int64
	}{
		{raw: "", want: 0},
		{raw: "123", want: 123},
		{raw: "1.2万", want: 12000},
		{raw: "3.5w", want: 35000},
		{raw: "1,234", want: 1234},
	}

	for _, tt := range tests {
		got, err := ParseCount(tt.raw)
		if err != nil {
			t.Fatalf("ParseCount(%q) error: %v", tt.raw, err)
		}
		if got != tt.want {
			t.Errorf("ParseCount(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestStoreAnalyzesGrowthAndCompetition(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	first := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)

	if _, err := store.SaveSnapshot(ctx, Snapshot{
		Keyword:     "咖啡",
		CollectedAt: first,
		Items: []SnapshotItem{
			{FeedID: "a", Title: "旧笔记", AuthorID: "u1", LikedCount: "100", CollectedCount: "20", CommentCount: "10", SharedCount: "5"},
			{FeedID: "b", Title: "旧笔记2", AuthorID: "u2", LikedCount: "50", CollectedCount: "10", CommentCount: "5", SharedCount: "2"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveSnapshot(ctx, Snapshot{
		Keyword:     "咖啡",
		CollectedAt: second,
		Items: []SnapshotItem{
			{FeedID: "c", Title: "新笔记", AuthorID: "u3", LikedCount: "200", CollectedCount: "40", CommentCount: "20", SharedCount: "10"},
			{FeedID: "d", Title: "新笔记2", AuthorID: "u3", LikedCount: "100", CollectedCount: "20", CommentCount: "10", SharedCount: "5"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := store.AnalyzeTopics(ctx, []string{"咖啡"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d analyses, want 1", len(results))
	}
	result := results[0]
	if !result.HasGrowthBaseline || result.GrowthRate <= 0 {
		t.Fatalf("growth = %v (baseline=%v), want positive growth with baseline", result.GrowthRate, result.HasGrowthBaseline)
	}
	if result.SampledPosts != 2 || result.UniqueAuthors != 1 {
		t.Fatalf("competition = posts:%d authors:%d, want posts:2 authors:1", result.SampledPosts, result.UniqueAuthors)
	}
	if len(result.TopTitles) == 0 || result.TopTitles[0] != "新笔记" {
		t.Fatalf("top titles = %#v, want 新笔记 first", result.TopTitles)
	}
}
