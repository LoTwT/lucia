package main

import "time"

// bubbleAllowed applies the user's bubble frequency preference. A negative
// cooldown disables bubbles; zero allows every reaction; a positive value
// requires the previous bubble to be old enough.
func bubbleAllowed(now, last time.Time, cooldownSeconds int) bool {
	if cooldownSeconds < 0 {
		return false
	}
	if cooldownSeconds == 0 || last.IsZero() {
		return true
	}
	return now.Sub(last) >= time.Duration(cooldownSeconds)*time.Second
}
