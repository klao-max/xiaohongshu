package main

import (
	"sync"

	"github.com/xpzouying/headless_browser"
)

// browserTaskGate 限制同一进程同时运行的浏览器任务数。
var browserTaskGate = newBrowserGate(1)

type browserGate struct {
	slots chan struct{}
}

func newBrowserGate(limit int) *browserGate {
	if limit < 1 {
		limit = 1
	}
	return &browserGate{slots: make(chan struct{}, limit)}
}

func (g *browserGate) acquire() func() {
	g.slots <- struct{}{}

	var once sync.Once
	return func() {
		once.Do(func() { <-g.slots })
	}
}

// managedBrowser 在关闭浏览器时释放并发槽位。
type managedBrowser struct {
	*headless_browser.Browser
	release func()
}

func (b *managedBrowser) Close() {
	if b == nil {
		return
	}
	if b.Browser != nil {
		b.Browser.Close()
	}
	if b.release != nil {
		b.release()
		b.release = nil
	}
}
