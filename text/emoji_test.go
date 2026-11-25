package text

import (
	"testing"
	"unicode"

	"github.com/marrasen/gunim/emoji"
)

// Every emoji there is shows as one, and the letter or symbol it is made of shows as text where Unicode says it does.
func TestWhichCharactersAreEmoji(t *testing.T) {
	for _, group := range emoji.Groups() {
		for _, e := range group.Emoji {
			runes := []rune(e.Text)
			if !drawsAsEmoji(runes) {
				t.Errorf("%s, %U, is not an emoji", e.Name, runes)
			}
			if len(runes) == 2 && runes[1] == emojiSelector && drawsAsEmoji(runes[:1]) {
				t.Errorf("%s, %U, is an emoji unasked", e.Name, runes[0])
			}
			if drawsAsEmoji([]rune{runes[0], textSelector}) {
				t.Errorf("%s, %U, is an emoji where it is asked for as text", e.Name, runes[0])
			}
		}
	}
}

// What is no emoji is none for asking.
func TestALetterIsNoEmojiForAsking(t *testing.T) {
	for _, s := range []string{"a\ufe0f", "é\ufe0f", "ä\ufe0f", "中\ufe0f", "א\ufe0f", "→\ufe0f", "x\U0001f3fb", "中\U0001f3fb", "1\ufe0f", "1\u20e3", "a\ufe0f\u20e3", "\u0301\ufe0f"} {
		if drawsAsEmoji([]rune(s)) {
			t.Errorf("%+q is an emoji", s)
		}
	}
	for r := rune(0xa0); r < 0x30000; r++ {
		if unicode.IsLetter(r) && r != 0x2139 && r != 0x24c2 && drawsAsEmoji([]rune{r, emojiSelector}) {
			t.Errorf("the letter %U is an emoji", r)
		}
	}
}
