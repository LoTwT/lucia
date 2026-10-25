package main

import (
	"testing"

	"github.com/LoTwT/lucia/internal/pet"
	"github.com/LoTwT/lucia/internal/settings"
	"github.com/egoist/mygo"
)

func TestThemeSourceForPreference(t *testing.T) {
	tests := []struct {
		value string
		want  mygo.ThemeSource
	}{
		{value: settings.ThemeSystem, want: mygo.ThemeSystem},
		{value: settings.ThemeLight, want: mygo.ThemeLight},
		{value: settings.ThemeDark, want: mygo.ThemeDark},
		{value: "", want: mygo.ThemeSystem},
		{value: "sepia", want: mygo.ThemeSystem},
	}
	for _, tt := range tests {
		if got := themeSourceForPreference(tt.value); got != tt.want {
			t.Errorf("themeSourceForPreference(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestBubbleThemeForPreference(t *testing.T) {
	tests := []struct {
		value string
		want  pet.BubbleTheme
	}{
		{value: settings.BubbleThemeApp, want: pet.BubbleThemeApp},
		{value: settings.BubbleThemeLight, want: pet.BubbleThemeLight},
		{value: settings.BubbleThemeDark, want: pet.BubbleThemeDark},
		{value: "", want: pet.BubbleThemeApp},
		{value: "sepia", want: pet.BubbleThemeApp},
	}
	for _, tt := range tests {
		if got := bubbleThemeForPreference(tt.value); got != tt.want {
			t.Errorf("bubbleThemeForPreference(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestBubbleCloseDoesNotCancelQuit(t *testing.T) {
	tests := []struct {
		name     string
		quitting func() bool
		prevent  bool
	}{
		{name: "normal close hides", prevent: true},
		{name: "quit closes", quitting: func() bool { return true }, prevent: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bubbleWindow{}
			e := &mygo.CloseEvent{}
			b.handleClose(e, tt.quitting)
			if got := e.DefaultPrevented(); got != tt.prevent {
				t.Fatalf("default prevented = %v, want %v", got, tt.prevent)
			}
		})
	}
}
