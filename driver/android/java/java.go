// Package java holds the Java half of gunim's Android driver: the
// activity, the view it draws in, and the soft keyboard's side of the
// text. tools/gunimapk compiles it into every APK it builds.
package java

import "embed"

// Sources holds the Java source files.
//
//go:embed *.java
var Sources embed.FS
