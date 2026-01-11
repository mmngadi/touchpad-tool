package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	executablePath, _ := os.Getwd()
	root, err := findModuleRoot(executablePath)
	if err != nil {
		fmt.Println("[-] Error: Could not find go.mod in any parent directory.")
		os.Exit(1)
	}

	err = os.Chdir(root)
	if err != nil {
		fmt.Printf("[-] Failed to switch to root: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Touchpad Build Pipeline: %s ===\n", runtime.GOOS)

	// Paths
	internalDir := filepath.Join(root, "internal")
	releaseDir := filepath.Join(root, "release")
	keystorePath := filepath.Join(internalDir, "touchpad.keystore")
	unpatchedAPK := filepath.Join(internalDir, "touchpad-unpatched.apk")
	patchedAPK := filepath.Join(internalDir, "touchpad-release.apk")
	touchpadPkg := "./internal/touchpad"

	// 0. Identity Check (Permanent Keystore)
	if _, err := os.Stat(keystorePath); os.IsNotExist(err) {
		fmt.Println("[0/4] Generating Permanent Keystore for consistent signing...")
		if err := generatePermanentKey(keystorePath); err != nil {
			fmt.Printf("[-] Keystore generation failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("[0/4] Using existing permanent keystore.")
	}

	// 1. Build APK
	fmt.Println("[1/4] Gomobile Build (API 30)...")
	cmdGomobile := exec.Command("gomobile", "build", "-androidapi", "30", "-o", unpatchedAPK, touchpadPkg)
	cmdGomobile.Dir = root
	if err := runCmd(cmdGomobile); err != nil {
		os.Exit(1)
	}

	// 2. Patch APK
	fmt.Println("[2/4] Applying Patch Logic...")
	cmdPatch := exec.Command("go", "run", "./internal/patch/main.go")
	cmdPatch.Dir = root
	if err := runCmd(cmdPatch); err != nil {
		os.Exit(1)
	}
	_ = os.Remove(unpatchedAPK)

	// 3. Build Tool
	// Ensure release dir exists
	_ = os.MkdirAll(releaseDir, 0755)
	binaryName := "touchpad-tool"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	outputPath := filepath.Join(releaseDir, binaryName)

	fmt.Printf("[3/4] Building Multi-OS Binary: %s\n", outputPath)
	// The binary embeds the APK located at patchedAPK during this step
	cmdTool := exec.Command("go", "build", "-o", outputPath, ".")
	cmdTool.Dir = root
	if err := runCmd(cmdTool); err != nil {
		os.Exit(1)
	}

	// 4. Finalize Release and Cleanup
	fmt.Println("[4/4] Cleaning up intermediate artifacts...")

	// SKEPTICAL FIX: Delete the APK after embedding it into the .exe
	// This ensures only the .exe remains in the release folder.
	err = os.Remove(patchedAPK)
	if err != nil {
		fmt.Printf("[!] Warning: Could not delete internal APK: %v\n", err)
	} else {
		fmt.Println("[+] Successfully removed internal APK after embedding.")
	}

	// Clean all .idsig files that build tools might generate
	files, _ := filepath.Glob(filepath.Join(internalDir, "*.idsig"))
	for _, f := range files {
		_ = os.Remove(f)
	}

	_ = os.Remove(filepath.Join(internalDir, "touchpad_aligned.apk"))

	fmt.Println("\n[SUCCESS] Build Finished.")
	fmt.Printf("[*] Final Binary (APK embedded): %s\n", outputPath)
}

func generatePermanentKey(ksPath string) error {
	pw := "password"
	cmd := exec.Command("keytool", "-genkey", "-v",
		"-keystore", ksPath,
		"-alias", "dev",
		"-keyalg", "RSA",
		"-keysize", "2048",
		"-validity", "10000",
		"-storepass", pw,
		"-keypass", pw,
		"-dname", "CN=Touchpad",
		"-noprompt")
	return cmd.Run()
}

func findModuleRoot(path string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return path, nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return "", fmt.Errorf("go.mod not found")
}

func runCmd(cmd *exec.Cmd) error {
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
