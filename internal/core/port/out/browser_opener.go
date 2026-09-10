package out

// BrowserOpener is the driven port used to launch the user's default web
// browser at a given URL. Kept as its own tiny port (rather than folded
// into the CLI adapter) because it is inherently OS-specific and the core
// should be able to run the login flow in a headless environment where
// opening a browser fails gracefully and the URL is simply printed instead.
type BrowserOpener interface {
	Open(url string) error
}
