package xiaohongshu

import "testing"

func TestCreatorLoginDetected(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		hasLoginPanel  bool
		wantLoginState bool
	}{
		{name: "creator home is logged in", url: "https://creator.xiaohongshu.com/new/home"},
		{name: "creator publish page is logged in", url: "https://creator.xiaohongshu.com/publish/publish"},
		{name: "login redirect", url: "https://creator.xiaohongshu.com/login?source=official", wantLoginState: true},
		{name: "login panel", url: "https://creator.xiaohongshu.com/new/home", hasLoginPanel: true, wantLoginState: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := creatorLoginDetected(tt.url, tt.hasLoginPanel); got != tt.wantLoginState {
				t.Fatalf("creatorLoginDetected(%q, %v) = %v, want %v", tt.url, tt.hasLoginPanel, got, tt.wantLoginState)
			}
		})
	}
}
