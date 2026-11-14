package main

import (
	"testing"
	"time"
)

func TestBrowserGateSerializesLeases(t *testing.T) {
	gate := newBrowserGate(1)
	releaseFirst := gate.acquire()

	acquiredSecond := make(chan struct{})
	go func() {
		releaseSecond := gate.acquire()
		close(acquiredSecond)
		releaseSecond()
	}()

	select {
	case <-acquiredSecond:
		t.Fatal("第二个浏览器任务不应在第一个释放前获取槽位")
	case <-time.After(50 * time.Millisecond):
	}

	releaseFirst()

	select {
	case <-acquiredSecond:
	case <-time.After(time.Second):
		t.Fatal("第一个浏览器任务释放后，第二个任务仍未获取槽位")
	}
}
