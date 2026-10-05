package main

import (
	"embed"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/band"
)

// The music is Greek Themes, a band of synths made in Reason at 136
// BPM, which package band plays: each synth comes in, loops and leaves
// in 16-bar phrases, and the brass plays its solo now and then.
// github.com/marrasen/gunim-music holds the same song, for any program.

// musicFiles are the synths' parts, made by music/encode.sh: each
// synth's intro, loop and outro, or its solo.
//
//go:embed music/*.ogg
var musicFiles embed.FS

// songBPM is the song's tempo.
const songBPM = 136

// song returns the song, its parts found in musicFiles.
func song() *band.Wander {
	parts, _ := band.Load(musicFiles, "music")
	return &band.Wander{Title: "Greek Themes", Artist: "Marcus Johansson", BPM: songBPM, Parts: parts}
}

// newMusic starts the band, seeded from the time, so each game's song is
// its own.
func newMusic() audio.Source {
	return song().Play(uint64(time.Now().UnixNano()))
}
