package drivers

// Define the interface here so the package knows what it's satisfying
type MouseDriver interface {
	Move(dx, dy int32)
	Button(button string, down bool)
	Scroll(delta int32)
	Close()
}
