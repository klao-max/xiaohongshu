package xiaohongshu

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const (
	urlOfCreatorHome       = `https://creator.xiaohongshu.com/?source=official`
	creatorSessionTimeout  = 20 * time.Second
	creatorSessionPollRate = 500 * time.Millisecond
)

// ensureCreatorSession 打开创作者中心首页，并根据最终页面判断登录状态。
func ensureCreatorSession(page *rod.Page) error {
	if err := page.Navigate(urlOfCreatorHome); err != nil {
		return errors.Wrap(err, "打开创作者中心失败")
	}

	_ = page.WaitLoad()

	deadline := time.Now().Add(creatorSessionTimeout)
	for time.Now().Before(deadline) {
		info, infoErr := page.Info()
		hasLoginPanel, _, panelErr := page.Has(".login-container")
		if infoErr == nil && panelErr == nil && creatorSessionReady(info.URL, hasLoginPanel) {
			humanize.Delay(page.GetContext(), humanize.AfterNavigate)
			return nil
		}
		time.Sleep(creatorSessionPollRate)
	}

	return errors.New("创作者中心登录失效，请重新扫码登录")
}

func creatorSessionReady(currentURL string, hasLoginPanel bool) bool {
	return !strings.Contains(currentURL, "/login") && !hasLoginPanel
}
