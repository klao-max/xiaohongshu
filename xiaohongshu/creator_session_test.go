package xiaohongshu

import "testing"

func TestCreatorSessionReady(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		hasLoginPanel bool
		wantReady     bool
	}{
		{name: "creator home is ready", url: "https://creator.xiaohongshu.com/new/home", wantReady: true},
		{name: "creator publish page is ready", url: "https://creator.xiaohongshu.com/publish/publish", wantReady: true},
		{name: "login redirect is pending", url: "https://creator.xiaohongshu.com/login?source=official"},
		{name: "transient login panel is pending", url: "https://creator.xiaohongshu.com/new/home", hasLoginPanel: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := creatorSessionReady(tt.url, tt.hasLoginPanel); got != tt.wantReady {
				t.Fatalf("creatorSessionReady(%q, %v) = %v, want %v", tt.url, tt.hasLoginPanel, got, tt.wantReady)
			}
		})
	}
}
