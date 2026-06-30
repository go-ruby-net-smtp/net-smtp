// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package netsmtp is a pure-Go (no cgo) reimplementation of the deterministic,
// interpreter-independent core of Ruby's Net::SMTP: the SMTP command grammar, the
// DATA message dot-stuffing, the reply-line parser, the EHLO capability parse,
// and the SASL auth encodings (PLAIN, LOGIN, CRAM-MD5). It mirrors MRI 4.0.5's
// net-smtp 0.5.1 byte-for-byte on every string it produces and every reply it
// parses, without any Ruby runtime.
//
// # What it is — and isn't
//
// Building a request line ("MAIL FROM:<a@b>"), dot-stuffing a DATA payload,
// splitting a "250-…/250 …" reply into a Response, decoding an EHLO capability
// list, and encoding an AUTH PLAIN/LOGIN/CRAM-MD5 exchange are all pure compute,
// so they live here. The TCP connection, the TLS upgrade (STARTTLS), and the
// blocking read/write of those bytes are the host's job: this package never opens
// a socket. The host (go-embedded-ruby's rbgo) supplies a Conn, and the Session
// driver below sequences the codec against it so the same exchange MRI performs
// over a real socket is reproduced here with the I/O factored out as a seam.
//
// # The socket / TLS seam
//
// A host wires in transport by implementing Conn:
//
//	type Conn interface {
//		WriteLine(line string) error // send a request line + CRLF
//		ReadLine() (string, error)   // read one reply line, CRLF stripped
//		WriteRaw(b string) error     // send a DATA payload verbatim
//		StartTLS() error             // upgrade the connection in place
//	}
//
// The codec functions (HELO, MailFrom, WriteMessage, ParseResponse, AuthPlain, …)
// are usable directly without a Conn; Session is the optional convenience driver.
package netsmtp
