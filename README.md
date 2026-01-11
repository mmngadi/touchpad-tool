# 📱 Android Touchpad Tool

Transform your Android device into a high-precision, low-latency PC touchpad. By piping raw events from the **Linux kernel digitizer** via ADB, this tool bypasses Android's UI layer to provide a native-feel mouse experience.

## 🛠 Prerequisites

You must set up both the Go environment and the Android cross-compilation toolchain.

### 1. The Basics

* **Go (Golang)**: [v1.18+](https://go.dev/dl/) installed and added to your `PATH`.
* **ADB (Android Debug Bridge)**: Part of the [SDK Platform Tools](https://developer.android.com/tools/releases/platform-tools). Ensure `adb devices` works in your terminal.
* **USB Debugging**: Enabled on your phone (Settings > Developer Options).

### 2. Mobile Build Toolchain

To compile the Go-native APK, you need the mobile bind toolset:

1. **Install Gomobile**:
```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init

```

2. **Android NDK**: `gomobile` requires the NDK to cross-compile Go to C-shared libraries for Android.
* Download it via Android Studio (SDK Manager > SDK Tools > NDK).
* Set your environment variable: `export ANDROID_NDK_HOME=/path/to/your/ndk` (or set it in Windows Environment Variables).

---

## 🏗 Setup & Build

### 1. Identify your Touch Device

Every phone assigns the screen digitizer to a different event node.

1. Connect your phone via USB.
2. Run: `adb shell getevent -p`
3. Touch the screen and look for the node (e.g., `/dev/input/event4`) that outputs `ABS_MT_POSITION_X`.
4. **Crucial**: Open `main.go` and update the `touchDevice` constant to match your device.

### 2. The Build Pipeline

The project uses a `prebuild.go` script to compile the mobile app and embed it into the desktop controller.

```bash
go run prebuild.go

```

* This compiles the code in `internal/touchpad/main.go` using `gomobile`.
* It generates the final executable in the **`release/`** folder.

---

## 💻 How to Use

### Windows

1. Open **PowerShell** or **CMD** as **Administrator**.
2. `cd release/`
3. `.\touchpad-tool.exe`

### Linux

1. Open terminal and `cd release/`.
2. `chmod +x touchpad-tool`
3. `sudo ./touchpad-tool`

### Mobile Activation

When the tool starts, watch your phone. **Google Play Protect** will block the install. Click **"More details"** > **"Install anyway"**. The screen will turn black—this is the "Safe Zone" for your touches.

---

## 🖐 Supported Gestures

The tool emulates a high-end HID touchpad.

| Gesture | Action |
| --- | --- |
| **Single Finger** | Move mouse cursor |
| **Single Tap** | Left Click |
| **Two-Finger Slide** | Vertical Scroll |
| **Long Press** | Right Click |
| **Double-Tap & Hold** | Drag & Drop |

---

## 📝 Note on the APK (Internal Logic)

The mobile component is a Go-native app using OpenGL. Its primary job is to swallow system gestures (like "Back" or "Home") so they don't interfere with your mouse movements.

```go
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

```

---

## 🛑 Exit & Cleanup

Simply press **`Ctrl+C`** in your PC terminal. The tool will:

1. Stop the mouse driver.
2. **Uninstall** the APK from your phone automatically.
3. Restore your phone's original orientation and brightness.

---
