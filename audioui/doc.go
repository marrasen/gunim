// Package audioui draws sound for audio applications: level meters and
// faders, a spectrum and a spectrogram, a waveform, and loudness
// readings and curves.
//
// Each piece keeps the state it shows and paints itself into a
// rectangle it is given, so an application lays the pieces out in its
// own nodes. The pieces that take input, such as the [Fader], are
// nodes themselves. Their colours are theme tokens, whose defaults suit
// a dark studio: near-black ground, teal for sound, amber for a reading
// near its mark and coral for one past it.
package audioui
