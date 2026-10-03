//go:build !android

package text

// fontCacheDir returns "", which leaves the font scanner to keep its
// index where it does by default.
func fontCacheDir() string { return "" }
