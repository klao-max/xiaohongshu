package xiaohongshu

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const (
	urlOfCreatorHome = `https://creator.xiaohongshu.com/?source=official`
)

// ensureCreatorSession 打开创作者中心首页，并根据最终页面判断登录状态。
func ensureCreatorSession(page *rod.Page) error {
	if err := page.Navigate(urlOfCreatorHome); err != nil {
		return errors.Wrap(err, "打开创作者中心失败")
	}

	if err := page.WaitLoad(); err != nil {
		return errors.Wrap(err, "等待创作者中心加载失败")
	}

	if err := page.WaitStable(time.Second); err != nil {
		logrus.Warnf("创作者中心页面未稳定，继续检查登录状态: %v", err)
	}

	info, err := page.Info()
	if err != nil {
		return errors.Wrap(err, "读取创作者中心页面地址失败")
	}

	hasLoginPanel, _, err := page.Has(".login-container")
	if err != nil {
		return errors.Wrap(err, "检查创作者中心登录页面失败")
	}

	if creatorLoginDetected(info.URL, hasLoginPanel) {
		return errors.New("创作者中心登录失效，请重新扫码登录")
	}

	humanize.Delay(page.GetContext(), humanize.AfterNavigate)
	return nil
}

func creatorLoginDetected(currentURL string, hasLoginPanel bool) bool {
	return strings.Contains(currentURL, "/login") || hasLoginPanel
}
