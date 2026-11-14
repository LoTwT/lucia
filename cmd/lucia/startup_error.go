package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func showStartupError(message string) {
	mygo.App.WhenReady(func() {
		// This path has no parent window. Use an ordinary window rather
		// than an application-modal alert during startup; the latter can
		// enter its modal loop without becoming visible on macOS.
		mygo.NewWindow(mygo.WindowOptions{
			Title: "无法启动 Lucia", Width: 520, Height: 260,
			DisableResize: true, DisableMinimize: true, DisableMaximize: true,
			Content: ui.View(func(c *ui.Context) { startupErrorView(c, message, mygo.App.Quit) }),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Printf("lucia: show startup error: %v", err)
	}
}

func startupErrorView(c *ui.Context, message string, quit func()) {
	ui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
		ui.Text(c, "素材目录无法使用").FontSize(18).Bold()
		ui.Text(c, message).Grow(1)
		ui.PrimaryButton(c, "退出").OnClick(quit)
	})
}
