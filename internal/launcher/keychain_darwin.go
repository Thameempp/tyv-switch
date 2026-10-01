//go:build darwin

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
)

// keychainPassword protects each profile's private keychain. It is a fixed
// value on purpose: the keychain's only job is isolation (so agy does not
// reach the user's login keychain and does not pop up "Keychain Not Found"),
// and the file lives in an owner-only directory. It adds no secrecy.
const keychainPassword = "tyv-profile-keychain"

const securityTool = "/usr/bin/security"

// ensureProfileKeychain makes sure <home>/Library/Keychains/login.keychain-db
// exists and is unlocked. agy stores its login in the macOS keychain through
// the `security` tool, which locates the keychain from $HOME; without one it
// shows a "Keychain Not Found" dialog. Failures are non-fatal: agy then falls
// back to file storage inside the profile's private .gemini.
func ensureProfileKeychain(home string) {
	dir := filepath.Join(home, "Library", "Keychains")
	kc := filepath.Join(dir, "login.keychain-db")
	env := append(os.Environ(), "HOME="+home)

	run := func(args ...string) error {
		cmd := exec.Command(securityTool, args...)
		cmd.Env = env
		return cmd.Run()
	}

	if _, err := os.Stat(kc); err != nil {
		if err := run("create-keychain", "-p", keychainPassword, kc); err != nil {
			debugLog("could not create profile keychain: %v", err)
			return
		}
		// No auto-lock timeout and no lock on sleep.
		_ = run("set-keychain-settings", kc)
	}
	_ = run("unlock-keychain", "-p", keychainPassword, kc)
}
