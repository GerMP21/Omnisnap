package install

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strings"
)

// Mozilla signs each release's SHA512SUMS with an OpenPGP detached signature
// (SHA512SUMS.asc) made by a subkey of the release key pinned in this binary
// (mozilla-key.asc). Verification is pure Go: a pinned key block, v4 RSA
// signatures, PKCS#1 v1.5. No gpg, no dependency, works on Windows.
//
// Source: RFC 4880 §5.2 (packets), §5.2.2 (v4 keys), §5.2.3 (v4 signatures);
// https://ftp.mozilla.org/pub/firefox/releases/<version>/KEY

// VerifyDetached checks an armored detached signature over data with an
// armored public key block. Any subkey may have signed.
func VerifyDetached(data, armoredSig, armoredKey []byte) error {
	keys, err := parseKeyring(armoredKey)
	if err != nil {
		return err
	}
	sigs, err := parseSignatures(armoredSig)
	if err != nil {
		return err
	}
	if len(sigs) == 0 {
		return errors.New("install: no signature packet")
	}
	var lastErr error
	for _, sig := range sigs {
		if sig.sigType != 0x00 { // binary document
			continue
		}
		pub, ok := keys[sig.issuer]
		if !ok {
			lastErr = fmt.Errorf("install: signature by unknown key %016X", sig.issuer)
			continue
		}
		if err := sig.verify(data, pub); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("install: no binary-document signature")
	}
	return lastErr
}

// armor decodes every ASCII-armored block of a document, skipping the prose
// around it (Mozilla's KEY file starts with paragraphs).
func armor(doc []byte) ([][]byte, error) {
	var out [][]byte
	s := string(doc)
	for {
		start := strings.Index(s, "-----BEGIN PGP ")
		if start < 0 {
			break
		}
		body := s[start:]
		nl := strings.Index(body, "\n")
		if nl < 0 {
			return nil, errors.New("install: armor: no body")
		}
		body = body[nl+1:]
		for { // header lines until the blank one
			line, rest, ok := strings.Cut(body, "\n")
			if !ok {
				return nil, errors.New("install: armor: unterminated")
			}
			body = rest
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		end := strings.Index(body, "-----END PGP ")
		if end < 0 {
			return nil, errors.New("install: armor: no end line")
		}
		var b64 strings.Builder
		for _, line := range strings.Split(body[:end], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "=") {
				continue // the checksum line
			}
			b64.WriteString(line)
		}
		raw, err := base64.StdEncoding.DecodeString(b64.String())
		if err != nil {
			return nil, fmt.Errorf("install: armor: %w", err)
		}
		out = append(out, raw)
		s = body[end:]
	}
	if len(out) == 0 {
		return nil, errors.New("install: armor: no PGP block")
	}
	return out, nil
}

// packet is one OpenPGP packet.
type packet struct {
	tag  int
	body []byte
}

// packets splits a binary OpenPGP stream (new-format headers: what Mozilla's
// tools emit).
func packets(b []byte) ([]packet, error) {
	var out []packet
	for len(b) > 0 {
		h := b[0]
		if h&0x80 == 0 {
			return nil, errors.New("install: openpgp: bad packet header")
		}
		var tag, n int
		var rest []byte
		var err error
		if h&0x40 == 0 {
			// Old-format header (RFC 4880 §4.2): tag in bits 2-5, a length
			// type in bits 0-1. Mozilla's KEY still contains some.
			tag = int(h>>2) & 0x0f
			r := b[1:]
			switch h & 3 {
			case 0:
				if len(r) < 1 {
					return nil, errors.New("install: openpgp: truncated length")
				}
				n, rest = int(r[0]), r[1:]
			case 1:
				if len(r) < 2 {
					return nil, errors.New("install: openpgp: truncated length")
				}
				n, rest = int(binary.BigEndian.Uint16(r[0:2])), r[2:]
			case 2:
				if len(r) < 4 {
					return nil, errors.New("install: openpgp: truncated length")
				}
				n, rest = int(binary.BigEndian.Uint32(r[0:4])), r[4:]
			default:
				n, rest = len(r), r // indeterminate: to the end
			}
		} else {
			tag = int(h & 0x3f)
			n, rest, err = subpacketLength(b[1:])
			if err != nil {
				return nil, err
			}
		}
		if n > len(rest) {
			return nil, errors.New("install: openpgp: packet exceeds data")
		}
		out = append(out, packet{tag: tag, body: rest[:n]})
		b = rest[n:]
	}
	return out, nil
}

// parseKeyring reads tag 6 (public key) and tag 14 (public subkey) packets.
func parseKeyring(doc []byte) (map[uint64]*rsa.PublicKey, error) {
	blocks, err := armor(doc)
	if err != nil {
		return nil, err
	}
	keys := map[uint64]*rsa.PublicKey{}
	for _, block := range blocks {
		ps, err := packets(block)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if p.tag != 6 && p.tag != 14 {
				continue
			}
			pub, id, err := parseRSAPublicKey(p.body)
			if err != nil {
				return nil, err
			}
			keys[id] = pub
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("install: openpgp: no RSA key in the key block")
	}
	return keys, nil
}

// parseRSAPublicKey reads a v4 RSA key packet and computes its key id
// (RFC 4880 §12.2: the last eight bytes of the fingerprint).
func parseRSAPublicKey(body []byte) (*rsa.PublicKey, uint64, error) {
	if len(body) < 6 || body[0] != 4 {
		return nil, 0, errors.New("install: openpgp: key is not v4")
	}
	if body[5] != 1 { // RSA (encrypt or sign)
		return nil, 0, fmt.Errorf("install: openpgp: key algorithm %d", body[5])
	}
	n, rest, err := mpi(body[6:])
	if err != nil {
		return nil, 0, err
	}
	e, _, err := mpi(rest)
	if err != nil {
		return nil, 0, err
	}
	fp := sha1.New()
	fp.Write([]byte{0x99, byte(len(body) >> 8), byte(len(body))})
	fp.Write(body)
	sum := fp.Sum(nil)
	id := binary.BigEndian.Uint64(sum[12:20])
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, id, nil
}

// signature is one parsed v4 signature packet.
type signature struct {
	sigType  byte
	hashAlgo byte
	issuer   uint64
	hashed   []byte // version..hashed subpackets, as hashed
	left16   uint16
	sig      []byte
}

func parseSignatures(doc []byte) ([]signature, error) {
	blocks, err := armor(doc)
	if err != nil {
		return nil, err
	}
	var out []signature
	for _, block := range blocks {
		ps, err := packets(block)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if p.tag != 2 {
				continue
			}
			sig, err := parseSignature(p.body)
			if err != nil {
				return nil, err
			}
			out = append(out, sig)
		}
	}
	return out, nil
}

func parseSignature(body []byte) (signature, error) {
	var sig signature
	if len(body) < 6 || body[0] != 4 {
		return sig, errors.New("install: openpgp: signature is not v4")
	}
	sig.sigType, sig.hashAlgo = body[1], body[3]
	hashedLen := int(binary.BigEndian.Uint16(body[4:6]))
	if 6+hashedLen > len(body) {
		return sig, errors.New("install: openpgp: hashed subpackets exceed the packet")
	}
	sig.hashed = body[:6+hashedLen]
	rest := body[6+hashedLen:]
	issuer, err := issuerOf(body[6 : 6+hashedLen])
	if err != nil {
		return sig, err
	}
	if len(rest) < 4 {
		return sig, errors.New("install: openpgp: truncated signature")
	}
	unhashedLen := int(binary.BigEndian.Uint16(rest[0:2]))
	if 2+unhashedLen+2+2 > len(rest) {
		return sig, errors.New("install: openpgp: unhashed subpackets exceed the packet")
	}
	if issuer == 0 {
		if issuer, err = issuerOf(rest[2 : 2+unhashedLen]); err != nil {
			return sig, err
		}
	}
	sig.issuer = issuer
	after := rest[2+unhashedLen:]
	sig.left16 = binary.BigEndian.Uint16(after[0:2])
	v, _, err := mpi(after[2:])
	if err != nil {
		return sig, err
	}
	sig.sig = v.Bytes()
	return sig, nil
}

// issuerOf finds subpacket type 16 (issuer key id).
func issuerOf(subpackets []byte) (uint64, error) {
	for len(subpackets) > 0 {
		size, rest, err := subpacketLength(subpackets)
		if err != nil {
			return 0, err
		}
		if size > len(rest) {
			return 0, errors.New("install: openpgp: subpacket exceeds data")
		}
		body := rest[:size]
		subpackets = rest[size:]
		if len(body) >= 9 && body[0] == 16 {
			return binary.BigEndian.Uint64(body[1:9]), nil
		}
	}
	return 0, nil
}

// subpacketLength decodes an OpenPGP length (packet or subpacket).
func subpacketLength(b []byte) (int, []byte, error) {
	if len(b) < 1 {
		return 0, nil, errors.New("install: openpgp: truncated length")
	}
	switch first := b[0]; {
	case first < 192:
		return int(first), b[1:], nil
	case first < 224:
		if len(b) < 2 {
			return 0, nil, errors.New("install: openpgp: truncated length")
		}
		return (int(first)-192)<<8 + int(b[1]) + 192, b[2:], nil
	case first == 255:
		if len(b) < 5 {
			return 0, nil, errors.New("install: openpgp: truncated length")
		}
		return int(binary.BigEndian.Uint32(b[1:5])), b[5:], nil
	default:
		return 0, nil, errors.New("install: openpgp: partial length (unsupported)")
	}
}

// verify checks the signature over data with pub.
func (s signature) verify(data []byte, pub *rsa.PublicKey) error {
	h, hc, err := hashFor(s.hashAlgo)
	if err != nil {
		return err
	}
	h.Write(data)
	h.Write(s.hashed)
	// RFC 4880 §5.2.4: trailer 0x04 0xff and the hashed area's length.
	trailer := []byte{0x04, 0xff, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(trailer[2:], uint32(len(s.hashed)))
	h.Write(trailer)
	digest := h.Sum(nil)
	if len(digest) < 2 || binary.BigEndian.Uint16(digest[0:2]) != s.left16 {
		return errors.New("install: openpgp: signature hash mismatch")
	}
	if err := rsa.VerifyPKCS1v15(pub, hc, digest, s.sig); err != nil {
		return fmt.Errorf("install: openpgp: %w", err)
	}
	return nil
}

func hashFor(algo byte) (hash.Hash, crypto.Hash, error) {
	switch algo {
	case 2:
		return sha1.New(), crypto.SHA1, nil
	case 8:
		return sha256.New(), crypto.SHA256, nil
	case 9:
		return sha512.New384(), crypto.SHA384, nil
	case 10:
		return sha512.New(), crypto.SHA512, nil
	case 11:
		return sha256.New224(), crypto.SHA224, nil
	}
	return nil, 0, fmt.Errorf("install: openpgp: hash algorithm %d", algo)
}

// mpi reads a multiprecision integer: 2-byte bit length, then bytes.
func mpi(b []byte) (*big.Int, []byte, error) {
	if len(b) < 2 {
		return nil, nil, errors.New("install: openpgp: truncated MPI")
	}
	bits := int(binary.BigEndian.Uint16(b[0:2]))
	n := (bits + 7) / 8
	if n > len(b)-2 {
		return nil, nil, errors.New("install: openpgp: MPI exceeds data")
	}
	return new(big.Int).SetBytes(b[2 : 2+n]), b[2+n:], nil
}
