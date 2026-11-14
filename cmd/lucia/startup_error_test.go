package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStartupErrorShowsReasonAndExitAction(t *testing.T) {
	const reason = "素材目录已被另一个进程占用，请等待导入结束或退出对应应用后重试"
	quit := false
	tester := ui.NewTester(func(c *ui.Context) {
		startupErrorView(c, reason, func() { quit = true })
	}, 520, 260)
	tester.Frame()
	if _, ok := tester.Find(reason); !ok {
		t.Fatal("startup error did not display the reason")
	}
	tester.Click("退出")
	tester.Frame()
	if !quit {
		t.Fatal("exit action did not run")
	}
}
