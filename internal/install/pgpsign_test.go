package install

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"strings"
	"testing"
	"time"
)

// testSigner signs at test time what the macOS and Windows lanes download: their
// fixture archives are built on the runner (hdiutil, WiX), so their
// SHA512SUMS cannot be signed ahead of time with a key kept in the repo. It
// writes the same packets the verifier reads (verify.go): a v4 RSA public key
// and a v4 binary-document signature over SHA-512, PKCS#1 v1.5.
// Source: RFC 4880 §4.2.2 (new-format lengths), §5.2.3 (v4 signatures),
// §5.2.4 (the hashed trailer), §5.5.2 (v4 keys), §12.2 (key id).
type testSigner struct {
	key  *rsa.PrivateKey
	body []byte // the public-key packet body
	id   uint64
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{4, 0, 0, 0, 0, 1} // v4, creation time, RSA
	binary.BigEndian.PutUint32(body[1:5], uint32(time.Now().Unix()))
	body = append(body, mpiBytes(key.N)...)
	body = append(body, mpiBytes(big.NewInt(int64(key.E)))...)
	fp := sha1.New()
	fp.Write([]byte{0x99, byte(len(body) >> 8), byte(len(body))})
	fp.Write(body)
	sum := fp.Sum(nil)
	return &testSigner{key: key, body: body, id: binary.BigEndian.Uint64(sum[12:20])}
}

// publicKey is the armored key block Options.Key takes.
func (s *testSigner) publicKey() []byte {
	return armored("PUBLIC KEY BLOCK", framePacket(6, s.body))
}

// sign returns an armored detached signature over data.
func (s *testSigner) sign(t *testing.T, data []byte) []byte {
	t.Helper()
	created := make([]byte, 4)
	binary.BigEndian.PutUint32(created, uint32(time.Now().Unix()))
	issuer := make([]byte, 8)
	binary.BigEndian.PutUint64(issuer, s.id)
	var sub []byte
	sub = append(sub, 5, 2) // signature creation time
	sub = append(sub, created...)
	sub = append(sub, 9, 16) // issuer key id
	sub = append(sub, issuer...)
	hashed := []byte{4, 0x00, 1, 10, byte(len(sub) >> 8), byte(len(sub))} // v4, binary, RSA, SHA-512
	hashed = append(hashed, sub...)

	h := sha512.New()
	h.Write(data)
	h.Write(hashed)
	trailer := []byte{0x04, 0xff, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(trailer[2:], uint32(len(hashed)))
	h.Write(trailer)
	digest := h.Sum(nil)
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA512, digest)
	if err != nil {
		t.Fatal(err)
	}
	body := append([]byte{}, hashed...)
	body = append(body, 0, 0)                 // no unhashed subpackets
	body = append(body, digest[0], digest[1]) // left 16 bits of the hash
	body = append(body, mpiBytes(new(big.Int).SetBytes(sig))...)
	return armored("SIGNATURE", framePacket(2, body))
}

// framePacket frames body as a new-format packet (RFC 4880 §4.2.2).
func framePacket(tag byte, body []byte) []byte {
	out := []byte{0xc0 | tag}
	switch n := len(body); {
	case n < 192:
		out = append(out, byte(n))
	case n < 8384:
		n -= 192
		out = append(out, byte(n>>8)+192, byte(n))
	default:
		out = append(out, 0xff, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	return append(out, body...)
}

func mpiBytes(v *big.Int) []byte {
	b := v.Bytes()
	return append([]byte{byte(v.BitLen() >> 8), byte(v.BitLen())}, b...)
}

// armored wraps raw in ASCII armor; the verifier skips the CRC line, so none
// is written.
func armored(kind string, raw []byte) []byte {
	var b strings.Builder
	b.WriteString("-----BEGIN PGP " + kind + "-----\n\n")
	enc := base64.StdEncoding.EncodeToString(raw)
	for len(enc) > 64 {
		b.WriteString(enc[:64] + "\n")
		enc = enc[64:]
	}
	b.WriteString(enc + "\n-----END PGP " + kind + "-----\n")
	return []byte(b.String())
}

// The signer and the verifier agree, and a tampered document fails: the
// signer is only trusted because this test holds on every OS.
func TestTestSignerMatchesTheVerifier(t *testing.T) {
	s := newTestSigner(t)
	doc := []byte("abc  mac/en-US/Firefox 999.0.dmg\n")
	sig := s.sign(t, doc)
	if err := VerifyDetached(doc, sig, s.publicKey()); err != nil {
		t.Fatalf("own signature rejected: %v", err)
	}
	if err := VerifyDetached(append([]byte("x"), doc...), sig, s.publicKey()); err == nil {
		t.Error("a tampered document verified")
	}
	if err := VerifyDetached(doc, sig, testKey(t)); err == nil {
		t.Error("verified with an unrelated key")
	}
}
