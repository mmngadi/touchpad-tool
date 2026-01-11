package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	root, _ := os.Getwd()
	internalPath := filepath.Join(root, "internal")

	originalAPK := filepath.Join(internalPath, "touchpad-unpatched.apk")
	outputAPK := filepath.Join(internalPath, "touchpad-release.apk")
	alignedAPK := filepath.Join(internalPath, "touchpad_aligned.apk")
	ksFile := filepath.Join(internalPath, "touchpad.keystore")

	// Dynamic Build Tools detection
	sdkVersion := "36.1.0"
	buildToolsPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk", "build-tools", sdkVersion)

	apksigner := filepath.Join(buildToolsPath, "apksigner.bat")
	zipalign := filepath.Join(buildToolsPath, "zipalign.exe")

	// Step 1: Patching
	fmt.Println("[*] Step 1: Brute-Force Patching to API 30...")
	if err := patchAPK(originalAPK, outputAPK); err != nil {
		fmt.Printf("[-] Patch failed: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Aligning
	fmt.Println("[*] Step 2: Aligning APK...")
	runCmd(zipalign, "-f", "4", outputAPK, alignedAPK)

	// SKEPTICAL FIX: Removed "Step 3: Generating Keystore" from here.
	// Logic: prebuild.go handles this now. We only check if it exists.
	if _, err := os.Stat(ksFile); os.IsNotExist(err) {
		fmt.Println("[!] Keystore missing in patcher, generating as fallback...")
		generateKey(ksFile)
	}

	// Step 4: Signing
	fmt.Println("[*] Step 4: Signing with permanent identity...")
	signWithApksigner(apksigner, alignedAPK, ksFile)

	// Final Move
	os.Remove(outputAPK)
	err := os.Rename(alignedAPK, outputAPK)
	if err != nil {
		fmt.Printf("[-] Rename failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[+] SUCCESS! Created: %s\n", outputAPK)
}

func patchAPK(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("could not open source APK %s: %w", src, err)
	}
	defer r.Close()

	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer w.Close()

	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, f := range r.File {
		if strings.HasPrefix(strings.ToUpper(f.Name), "META-INF/") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		fw, err := zw.Create(f.Name)
		if err != nil {
			rc.Close()
			return err
		}

		if f.Name == "AndroidManifest.xml" {
			data, _ := io.ReadAll(rc)
			oldPattern := []byte{0x08, 0x00, 0x00, 0x10, 0x10, 0x00, 0x00, 0x00}
			newPattern := []byte{0x08, 0x00, 0x00, 0x10, 0x1E, 0x00, 0x00, 0x00}

			patchedData := bytes.ReplaceAll(data, oldPattern, newPattern)
			if bytes.Equal(data, patchedData) {
				fmt.Println("[!] Warning: Pattern for API 16 not found!")
			} else {
				fmt.Println("[+] Successfully patched binary XML API patterns to version 30.")
			}
			fw.Write(patchedData)
		} else {
			io.Copy(fw, rc)
		}
		rc.Close()
	}
	return nil
}

func generateKey(ks string) {
	pw := "password"
	runCmd("keytool", "-genkey", "-v", "-keystore", ks, "-alias", "dev", "-keyalg", "RSA", "-keysize", 2048, "-validity", "10000", "-storepass", pw, "-keypass", pw, "-dname", "CN=Touchpad", "-noprompt", "-deststoretype", "pkcs12")
}

func signWithApksigner(bin, apk, ks string) {
	// Note: output must be to a different name or same name depending on version.
	// apksigner sign --out is generally safer.
	runCmd(bin, "sign", "--ks", ks, "--ks-pass", "pass:password", "--ks-key-alias", "dev", "--out", apk, apk)
}

func runCmd(name string, args ...interface{}) {
	strArgs := make([]string, len(args))
	for i, v := range args {
		strArgs[i] = fmt.Sprint(v)
	}
	cmd := exec.Command(name, strArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[-] %s failed\n%s\n", name, string(out))
		os.Exit(1)
	}
}
