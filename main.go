package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mmngadi/touchpad-tool/internal/drivers"
)

//go:embed internal/touchpad-release.apk
var touchpadAPK []byte

const (
	pkgName      = "org.golang.todo.touchpad"
	activityName = "org.golang.app.GoNativeActivity"
	touchDevice  = "/dev/input/event4"

	sensitivity      = 3.2
	scrollSens       = 120
	tapTimeout       = 200 * time.Millisecond
	doubleTapTimeout = 250 * time.Millisecond
	longPressTimeout = 600 * time.Millisecond
)

var (
	driver          drivers.MouseDriver
	adbPath         = "adb"
	appInForeground = true
	isExiting       = false
	inputCmd        *exec.Cmd
)

func init() {
	if runtime.GOOS == "windows" {
		localADB := filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk", "platform-tools", "adb.exe")
		if _, err := os.Stat(localADB); err == nil {
			adbPath = localADB
		}
	}
}

func main() {
	driver = drivers.InitDriver()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	tmpAPK := filepath.Join(os.TempDir(), "touchpad.apk")
	if err := os.WriteFile(tmpAPK, touchpadAPK, 0644); err != nil {
		fmt.Printf("[-] Failed to write temporary APK: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[*] Touchpad Tool Active: Focused Watchdog Mode\n")

	setupEnvironment()

	fmt.Println("[*] Installing and Launching App...")
	_ = exec.Command(adbPath, "install", "-r", tmpAPK).Run()
	launchApp()

	go startForegroundWatcher()
	go startKioskWatchdog()
	go startProcessDeathWatcher(sigChan)

	fmt.Println("[*] Listening for events on " + touchDevice)
	go processInput()

	<-sigChan
	isExiting = true
	cleanup(tmpAPK)
}

func setupEnvironment() {
	runADB("shell", "settings", "put", "system", "accelerometer_rotation", "0")
	runADB("shell", "settings", "put", "system", "user_rotation", "3")
	runADB("shell", "settings", "put", "global", "policy_control", "immersive.full=sticky:*")
	runADB("shell", "settings", "put", "secure", "immersive_mode_confirmations", "confirmed")
	runADB("shell", "svc", "power", "stayon", "true")
}

func launchApp() {
	// Added -f 0x10000000 (FLAG_ACTIVITY_NEW_TASK) to allow the background script to force the UI to the front.
	runADB("shell", "am", "start", "-n", pkgName+"/"+activityName, "-f", "0x10000000")
}

func startForegroundWatcher() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		if isExiting {
			return
		}
		out, err := exec.Command(adbPath, "shell", "dumpsys", "window", "displays", "|", "grep", "mCurrentFocus").Output()
		if err == nil {
			appInForeground = strings.Contains(string(out), pkgName)
		}
	}
}

func startKioskWatchdog() {
	for {
		if isExiting {
			return
		}
		if !appInForeground {
			fmt.Println("[!] Focus lost. Re-applying orientation and returning to app...")
			runADB("shell", "settings", "put", "system", "user_rotation", "3")
			launchApp()
			// Added a small sleep to prevent the "Focus lost" log spam while the app is transitioning
			time.Sleep(1 * time.Second)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func startProcessDeathWatcher(sigChan chan os.Signal) {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		if isExiting {
			return
		}
		out, _ := exec.Command(adbPath, "shell", "pidof", pkgName).Output()
		if len(strings.TrimSpace(string(out))) == 0 {
			fmt.Println("\n[!] App process manually closed. Exiting...")
			select {
			case sigChan <- syscall.SIGTERM:
			default:
			}
			return
		}
	}
}

func processInput() {
	inputCmd = exec.Command(adbPath, "shell", "getevent", "-l")
	stdout, _ := inputCmd.StdoutPipe()
	inputCmd.Start()

	scanner := bufio.NewScanner(stdout)
	rePos := regexp.MustCompile(`ABS_MT_POSITION_(X|Y)\s+([0-9a-fA-F]+)`)

	var lastX, lastY, curX, curY int
	var touchStartTime time.Time
	var lastReleaseTime time.Time
	var hasMoved, rightClickDone, isDragging bool
	var lastTapWasPure bool
	var rightClickTimer *time.Timer
	var scrollAccum float64
	activeFingers := 0

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "ABS_MT_TRACKING_ID") {
			if strings.Contains(line, "ffffffff") {
				if activeFingers > 0 {
					activeFingers--
				}
				if isDragging {
					driver.Button("left", false)
					isDragging = false
				}
				lastTapWasPure = !hasMoved && activeFingers == 0
				lastReleaseTime = time.Now()
			} else {
				activeFingers++
				lastX, lastY = 0, 0
				if lastTapWasPure && time.Since(lastReleaseTime) < doubleTapTimeout {
					isDragging = true
					driver.Button("left", true)
				}
			}
		}

		if strings.Contains(line, "BTN_TOUCH") {
			if strings.Contains(line, " DOWN") {
				touchStartTime = time.Now()
				hasMoved, rightClickDone = false, false
				if !isDragging {
					rightClickTimer = time.AfterFunc(longPressTimeout, func() {
						if !hasMoved && !rightClickDone && activeFingers == 1 {
							driver.Button("right", true)
							driver.Button("right", false)
							rightClickDone = true
						}
					})
				}
			} else if strings.Contains(line, " UP") {
				if rightClickTimer != nil {
					rightClickTimer.Stop()
				}
				if !isDragging && !hasMoved && !rightClickDone && time.Since(touchStartTime) < tapTimeout {
					driver.Button("left", true)
					driver.Button("left", false)
				}
			}
		}

		if strings.Contains(line, touchDevice) {
			if matches := rePos.FindStringSubmatch(line); len(matches) > 0 {
				val, _ := strconv.ParseInt(matches[2], 16, 64)
				if matches[1] == "X" {
					curX = int(val)
				} else {
					curY = int(val)
				}
			}

			if strings.Contains(line, "SYN_REPORT") {
				if !appInForeground {
					lastX, lastY = 0, 0
					continue
				}

				if lastX != 0 && lastY != 0 && curX != 0 && curY != 0 {
					dx := int32(float64(lastY-curY) * sensitivity)
					dy := int32(float64(curX-lastX) * sensitivity)

					if dx != 0 || dy != 0 {
						hasMoved = true
						if rightClickTimer != nil {
							rightClickTimer.Stop()
						}

						if activeFingers >= 2 {
							scrollAccum += float64(dy) * 0.1
							if scrollAccum >= 1.0 || scrollAccum <= -1.0 {
								driver.Scroll(int32(scrollAccum * float64(scrollSens)))
								scrollAccum = 0
							}
						} else {
							driver.Move(dx, dy)
						}
					}
				}
				lastX, lastY = curX, curY
			}
		}
	}
}

func runADB(args ...string) { _ = exec.Command(adbPath, args...).Run() }

func cleanup(tmpPath string) {
	fmt.Println("\n[*] Restoring Device Settings...")
	if inputCmd != nil && inputCmd.Process != nil {
		_ = inputCmd.Process.Kill()
	}
	if driver != nil {
		driver.Close()
	}
	runADB("shell", "settings", "put", "system", "accelerometer_rotation", "1")
	runADB("shell", "settings", "put", "global", "policy_control", "null")
	runADB("shell", "svc", "power", "stayon", "false")
	runADB("shell", "am", "force-stop", pkgName)
	runADB("uninstall", pkgName)
	_ = os.Remove(tmpPath)
	fmt.Println("[+] Done.")
	os.Exit(0)
}
