package zipcrypt

import (
	"archive/zip"
	"compress/flate"
	"hash"
	"hash/crc32"
	"io"
)

// legacyHeader is how many bytes of random data and check come before
// what an entry the old protection protects holds.
const legacyHeader = 12

// legacyKeys are the three keys of PKWARE's old protection, which every
// byte decrypted moves on.
type legacyKeys [3]uint32

func newLegacyKeys(password string) *legacyKeys {
	k := &legacyKeys{0x12345678, 0x23456789, 0x34567890}
	for i := range len(password) {
		k.update(password[i])
	}
	return k
}

func (k *legacyKeys) update(b byte) {
	k[0] = crc32.IEEETable[byte(k[0])^b] ^ k[0]>>8
	k[1] = (k[1]+k[0]&0xff)*134775813 + 1
	k[2] = crc32.IEEETable[byte(k[2])^byte(k[1]>>24)] ^ k[2]>>8
}

// stream is the byte the keys hide the next one behind.
func (k *legacyKeys) stream() byte {
	t := k[2] | 2
	return byte((t * (t ^ 1)) >> 8)
}

func (k *legacyKeys) decrypt(p []byte) {
	for i, c := range p {
		p[i] = c ^ k.stream()
		k.update(p[i])
	}
}

// openLegacy opens f, which the old protection protects, its bytes as
// they are stored coming from raw.
func openLegacy(f *zip.File, raw io.Reader, password string) (io.ReadCloser, error) {
	if f.CompressedSize64 < legacyHeader {
		return nil, ErrDamaged
	}
	var stored [legacyHeader]byte
	if _, err := io.ReadFull(raw, stored[:]); err != nil {
		return nil, err
	}
	// The last byte of the header is the high byte of the CRC, or of the
	// time where a data descriptor carries the CRC after the data.
	check := byte(f.CRC32 >> 24)
	if f.Flags&flagDescriptor != 0 {
		check = byte(f.ModifiedTime >> 8)
	}
	var keys *legacyKeys
	for _, form := range legacyForms(password) {
		k, head := newLegacyKeys(form), stored
		k.decrypt(head[:])
		if head[legacyHeader-1] == check {
			keys = k
			break
		}
	}
	if keys == nil {
		return nil, ErrPassword
	}
	body := &legacyReader{in: io.LimitReader(raw, int64(f.CompressedSize64-legacyHeader)), keys: keys}
	out, err := decompress(f.Method, body)
	if err != nil {
		return nil, err
	}
	return &checked{in: out, crc: crc32.NewIEEE(), want: f.CRC32, size: f.UncompressedSize64}, nil
}

// legacyReader decrypts what in reads.
type legacyReader struct {
	in   io.Reader
	keys *legacyKeys
}

func (r *legacyReader) Read(p []byte) (int, error) {
	n, err := r.in.Read(p)
	r.keys.decrypt(p[:n])
	return n, err
}

// checked reads in, and at its end checks the CRC of what it read, and
// its size, against want and size.
type checked struct {
	in   io.ReadCloser
	crc  hash.Hash32
	want uint32
	size uint64
	read uint64
	// noCRC leaves the CRC unchecked, for an entry that does not store
	// one.
	noCRC bool
	// end, when set, checks more once in is read to its end.
	end func() error
}

func (c *checked) Read(p []byte) (int, error) {
	n, err := c.in.Read(p)
	c.crc.Write(p[:n])
	c.read += uint64(n)
	if err == io.EOF {
		if c.read != c.size || !c.noCRC && c.crc.Sum32() != c.want {
			return n, ErrDamaged
		}
		if c.end != nil {
			if eerr := c.end(); eerr != nil {
				return n, eerr
			}
		}
	}
	return n, err
}

// Close closes what checked reads.
func (c *checked) Close() error { return c.in.Close() }

// flateReader returns what in holds, compressed with Deflate.
func flateReader(in io.Reader) io.ReadCloser { return flate.NewReader(in) }
