package main

import (
	"time"

	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/gl"
)

func main() {
	app.Main(func(a app.App) {
		var glctx gl.Context
		var lastBackPress time.Time
		var flashUntil time.Time // Used for visual feedback on first press

		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				glctx, _ = e.DrawContext.(gl.Context)

			case key.Event:
				// Raw Android KeyCode for Back is 4
				// We check the e.Code or the raw event if available
				if (e.Code == 4 || e.Code == key.CodeEscape) && e.Direction == key.DirRelease {
					now := time.Now()
					if now.Sub(lastBackPress) < 2*time.Second {
						return // Double-tap detected: Exit
					}
					lastBackPress = now
					flashUntil = now.Add(150 * time.Millisecond)
					a.Send(paint.Event{})
					continue // BLOCK the event from reaching the OS
				}

			case paint.Event:
				if glctx == nil {
					continue
				}

				// Visual Feedback: Flash gray if we just pressed back, otherwise black
				if time.Now().Before(flashUntil) {
					glctx.ClearColor(0.2, 0.2, 0.2, 1) // Dark Gray
				} else {
					glctx.ClearColor(0, 0, 0, 1) // Black
				}

				glctx.Clear(gl.COLOR_BUFFER_BIT)
				a.Publish()

				// If we are flashing, keep repainting until the flash duration ends
				if time.Now().Before(flashUntil) {
					a.Send(paint.Event{})
				}
			}
		}
	})
}
