// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
)

// base64M encodes like Ruby's String#pack('m0'): standard base64, padded, no line
// breaks. Net::SMTP::Authenticator#base64_encode uses exactly pack('m0').
func base64M(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// decodeBase64M decodes like Ruby's String#unpack1('m'): standard base64 that
// ignores characters outside the alphabet and tolerates missing padding. It is
// used for the CRAM-MD5 challenge (Response#cram_md5_challenge / the
// authenticator's challenge.unpack1('m')).
func decodeBase64M(s string) ([]byte, error) {
	// pack/unpack 'm' skips non-alphabet bytes and stops cleanly at incomplete
	// trailing groups; RawStdEncoding with the input's padding stripped mirrors
	// that leniency for the well-formed challenges servers actually send.
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isBase64Char(c) {
			clean = append(clean, c)
		}
	}
	// Drop any '=' padding we just kept, then decode raw.
	end := len(clean)
	for end > 0 && clean[end-1] == '=' {
		end--
	}
	return base64.RawStdEncoding.DecodeString(string(clean[:end]))
}

func isBase64Char(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '+' || c == '/' || c == '='
}

// AuthPlain builds the single AUTH PLAIN request line, mirroring AuthPlain#auth:
// finish('AUTH PLAIN ' + base64_encode("\0user\0secret")).
func AuthPlain(user, secret string) string {
	return "AUTH PLAIN " + base64M([]byte("\x00"+user+"\x00"+secret))
}

// AuthLoginInitial is the first AUTH LOGIN line (AuthLogin#auth: continue('AUTH LOGIN')).
func AuthLoginInitial() string { return "AUTH LOGIN" }

// AuthLoginUser is the base64-encoded user name sent after the username prompt
// (AuthLogin#auth: continue(base64_encode(user))).
func AuthLoginUser(user string) string { return base64M([]byte(user)) }

// AuthLoginSecret is the base64-encoded secret sent after the password prompt
// (AuthLogin#auth: finish(base64_encode(secret))).
func AuthLoginSecret(secret string) string { return base64M([]byte(secret)) }

// AuthCramMD5Initial is the first CRAM-MD5 line (AuthCramMD5#auth:
// continue('AUTH CRAM-MD5')), which prompts the server for its challenge.
func AuthCramMD5Initial() string { return "AUTH CRAM-MD5" }

// AuthCramMD5Response builds the base64 "user hexdigest" response to a decoded
// CRAM-MD5 challenge, mirroring AuthCramMD5#auth's final finish(...). The caller
// supplies the already-base64-decoded challenge bytes (the host's seam reads the
// server's challenge line; Response.CRAMMD5Challenge decodes it).
func AuthCramMD5Response(user, secret string, challenge []byte) string {
	crammed := cramMD5Response([]byte(secret), challenge)
	return base64M([]byte(user + " " + crammed))
}

// cramMD5Response is HMAC-MD5(secret, challenge) rendered as lowercase hex,
// reproducing AuthCramMD5#cram_md5_response byte-for-byte: the manual ipad/opad
// construction (which is precisely HMAC-MD5).
func cramMD5Response(secret, challenge []byte) string {
	inner := md5.Sum(append(cramSecret(secret, 0x36), challenge...))
	outer := md5.Sum(append(cramSecret(secret, 0x5c), inner[:]...))
	return hex.EncodeToString(outer[:])
}

const cramBufsize = 64

// cramSecret XORs the (optionally pre-hashed) secret, right-padded to 64 bytes,
// with the given mask, mirroring AuthCramMD5#cram_secret. A secret longer than 64
// bytes is first replaced by its MD5 digest.
func cramSecret(secret []byte, mask byte) []byte {
	if len(secret) > cramBufsize {
		d := md5.Sum(secret)
		secret = d[:]
	}
	buf := make([]byte, cramBufsize)
	copy(buf, secret)
	for i := range buf {
		buf[i] ^= mask
	}
	return buf
}
