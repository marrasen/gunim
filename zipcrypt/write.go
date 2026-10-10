package zipcrypt

import (
	"archive/zip"
	"compress/flate"
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"hash"
	"io"
	"slices"
	"strings"
	"sync"
)

// Create adds an entry to zw as zw.CreateHeader does, and protects what
// is written to it with password, by WinZip's AES with a 256-bit key,
// compressed with Deflate. It writes AE-1, which stores the CRC, as
// zw writes one for every entry. A folder, whose name ends in a slash,
// holds nothing, and goes in as it is. An empty password protects
// nothing, and the entry goes in compressed as fh says.
func Create(zw *zip.Writer, fh *zip.FileHeader, password string) (io.Writer, error) {
	if password == "" || strings.HasSuffix(fh.Name, "/") {
		return zw.CreateHeader(fh)
	}
	// Registered for each entry, so each entry has the password it is
	// created with.
	zw.RegisterCompressor(methodAES, func(w io.Writer) (io.WriteCloser, error) {
		return &aesWriter{out: w, password: password}, nil
	})
	fh.Method = methodAES
	fh.Flags |= flagEncrypted
	var extra [4 + aesExtraLen]byte
	binary.LittleEndian.PutUint16(extra[0:], aesExtraID)
	binary.LittleEndian.PutUint16(extra[2:], aesExtraLen)
	binary.LittleEndian.PutUint16(extra[4:], ae1)
	copy(extra[6:], "AE")
	extra[8] = aes256
	binary.LittleEndian.PutUint16(extra[9:], zip.Deflate)
	fh.Extra = append(fh.Extra, extra[:]...)
	return zw.CreateHeader(fh)
}

// aesWriter compresses what is written to it, and encrypts and signs it
// into out, after the salt and the password check. zip.Writer makes it
// before it writes the entry's header, so nothing goes to out until the
// first write, or the close.
type aesWriter struct {
	out      io.Writer
	password string
	flate    *flate.Writer
	ctr      *winZipCTR
	mac      hash.Hash
	started  bool
	err      error
	once     sync.Once
}

// start writes the salt and the password check.
func (w *aesWriter) start() error {
	if w.started || w.err != nil {
		return w.err
	}
	w.started = true
	spec := aesSpec{strength: aes256}
	salt := make([]byte, spec.saltLen())
	if _, err := rand.Read(salt); err != nil {
		w.err = err
		return err
	}
	enc, macKey, check, err := aesKeys(w.password, salt, spec.keyLen())
	if err != nil {
		w.err = err
		return err
	}
	block, err := aes.NewCipher(enc)
	if err != nil {
		w.err = err
		return err
	}
	w.ctr, w.mac = newWinZipCTR(block), hmac.New(sha1.New, macKey)
	if _, err := w.out.Write(slices.Concat(salt, check)); err != nil {
		w.err = err
		return err
	}
	w.flate, w.err = flate.NewWriter(sealer{w}, flate.DefaultCompression)
	return w.err
}

func (w *aesWriter) Write(p []byte) (int, error) {
	if err := w.start(); err != nil {
		return 0, err
	}
	return w.flate.Write(p)
}

// Close ends the compressed stream and writes the signature after it.
func (w *aesWriter) Close() error {
	if err := w.start(); err != nil {
		return err
	}
	err := errors.New("zipcrypt: closed twice")
	w.once.Do(func() {
		if err = w.flate.Close(); err != nil {
			return
		}
		_, err = w.out.Write(w.mac.Sum(nil)[:aesMACLen])
	})
	return err
}

// sealer encrypts and signs what the compressor writes, into the
// writer's out.
type sealer struct{ w *aesWriter }

func (s sealer) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	s.w.ctr.xor(buf)
	s.w.mac.Write(buf)
	return s.w.out.Write(buf)
}
