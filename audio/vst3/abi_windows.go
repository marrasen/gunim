package vst3

// comCompatible says the platform lays IDs out as COM GUIDs, as Windows
// does.
const comCompatible = true

// The result codes, as COM's.
const (
	resultNoInterface    = int32(-2147467262) // 0x80004002
	resultNotImplemented = int32(-2147467263) // 0x80004001
	resultInvalid        = int32(-2147024809) // 0x80070057
)
