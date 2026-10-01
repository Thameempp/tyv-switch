//go:build !darwin

package launcher

// ensureProfileKeychain is only needed on macOS.
func ensureProfileKeychain(string) {}
