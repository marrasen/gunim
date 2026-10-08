package driver

// A Compass is a [Window] that can tell which way the device faces, as a phone's compass does. Where the device has
// the sensors, WatchHeading(true) starts them, and the window's Input carries an
// [github.com/marrasen/gunim/input.Heading] at each reading, until WatchHeading(false) stops them again. The
// sensors cost battery while they run, so the engine runs them only while a node watches. HasCompass reports whether
// the device has the sensors; where it has none, WatchHeading does nothing. Both may be called from any goroutine.
type Compass interface {
	HasCompass() bool
	WatchHeading(on bool)
}
