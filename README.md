<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-net-smtp/brand/main/social/go-ruby-net-smtp-net-smtp.png" alt="go-ruby-net-smtp/net-smtp" width="720"></p>

# net-smtp — go-ruby-net-smtp

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-net-smtp.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Ruby's
[`Net::SMTP`](https://docs.ruby-lang.org/en/master/Net/SMTP.html)** — the SMTP
command grammar, the DATA message dot-stuffing, the reply-line parser, the EHLO
capability parse, and the SASL auth encodings (PLAIN, LOGIN, CRAM-MD5). It mirrors
MRI 4.0.5's `net-smtp` 0.5.1 **byte-for-byte** on every string it produces and
every reply it parses — **without any Ruby runtime**.

It is the SMTP backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo engine),
[go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler), and
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych codec).

> **What it is — and isn't.** Building a request line (`MAIL FROM:<a@b>`),
> dot-stuffing a `DATA` payload, splitting a `250-…/250 …` reply into a
> `Response`, decoding an EHLO capability list, and encoding an AUTH
> PLAIN/LOGIN/CRAM-MD5 exchange are all **pure compute**, so they live here. The
> TCP connection, the **TLS upgrade** (`STARTTLS`), and the blocking read/write of
> those bytes are the **host's job** — the host (rbgo) supplies a small `Conn`
> seam, and the `Session` driver sequences the codec against it. This library
> never opens a socket.

## Features

Faithful port of `Net::SMTP`'s protocol layer, validated against the `ruby`
binary on every supported platform:

- **Command builders** — `HELO` / `EHLO` / `MAIL FROM:<>` / `RCPT TO:<>` / `DATA`
  / `RSET` / `QUIT` / `STARTTLS`, each returning the exact reqline `getok` would
  send, with `Net::SMTP::Address`-faithful ESMTP-parameter joining and `#uniq`
  dedup, and the `validate_line` bare-CR/LF rejection.
- **DATA dot-stuffing** — the precise `InternetMessageIO#write_message`
  semantics: every line normalised to CRLF (`\n`, bare `\r`, and `\r\n` all
  handled), a leading `.` doubled, an unterminated final line CRLF-terminated, an
  empty source emitted as a lone CRLF, then the `.\r\n` terminator.
- **Reply parser** — `Net::SMTP::Response`: three-digit `Status`, full reply
  `String`, `Success` (2xx), `Continue` (3xx), `Message` (first line),
  `CRAMMD5Challenge`, and the multi-line `250-`/`250 ` continuation assembly.
- **EHLO capability parse** — `Response#capabilities`: the `verb → args` map from
  the continuation lines.
- **Error-class selection** — `Response#exception_class` keyed by code:
  `4xx → SMTPServerBusy`, `50x → SMTPSyntaxError`, `53x → SMTPAuthenticationError`,
  `5xx → SMTPFatalError`, else `SMTPUnknownError`, surfaced as an `ErrorKind` the
  host maps to the Ruby exception class.
- **SASL auth encoding** — `AUTH PLAIN` (base64 `\0user\0secret`), `AUTH LOGIN`
  (base64 user / pass), and `AUTH CRAM-MD5` (the HMAC-MD5 challenge response,
  pure given the decoded challenge — the RFC 2195 `tim`/`tanstaaftanstaaf` vector
  is in the suite), with the `>64`-byte secret pre-hash path.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) and three OSes (Linux, macOS, Windows).

## Install

```sh
go get github.com/go-ruby-net-smtp/net-smtp
```

## Usage

The codec functions are usable standalone:

```go
package main

import (
	"fmt"

	netsmtp "github.com/go-ruby-net-smtp/net-smtp"
)

func main() {
	line, _ := netsmtp.MailFrom("alice@example.com", "SIZE=4096")
	fmt.Printf("%q\n", netsmtp.Wire(line)) // "MAIL FROM:<alice@example.com> SIZE=4096\r\n"

	fmt.Printf("%q\n", netsmtp.WriteMessage(".hi\r\n")) // "..hi\r\n.\r\n"

	res := netsmtp.ParseResponse("250-PIPELINING\n250 AUTH PLAIN LOGIN\n")
	fmt.Println(res.Success(), res.Capabilities()) // true map[AUTH:[PLAIN LOGIN]]

	fmt.Println(netsmtp.AuthPlain("user", "pass")) // AUTH PLAIN AHVzZXIAcGFzcw==
}
```

## The socket / TLS seam

The host wires in transport by implementing `Conn`; the `Session` driver then
sequences the codec the way `Net::SMTP` does (`validate_line`, `writeline`,
`recv_response`, `check_response`):

```go
type Conn interface {
	WriteLine(line string) error // send a request line; the impl appends CRLF
	ReadLine() (string, error)   // read one reply line, CRLF stripped
	WriteRaw(b string) error     // send a DATA payload verbatim
	StartTLS() error             // upgrade the connection in place
}

s := netsmtp.NewSession(conn)
s.Ehlo("me.example.com")
s.AuthCramMD5("user", "secret") // drives AUTH CRAM-MD5 over the seam
s.MailFrom("a@b.com")
s.RcptTo("c@d.com")
s.Data("Subject: hi\r\n\r\nbody\r\n")
s.Quit()
```

`go-embedded-ruby`'s `rbgo` binds these: the pure codec produces every byte and
parses every reply, and rbgo provides the `Conn` (the TCP/TLS socket
`Net::SMTP` holds in `@socket`).

## API

```go
// Commands — return the bare reqline (validate_line'd); Wire appends CRLF.
func HELO(domain string) (string, error)
func EHLO(domain string) (string, error)
func MailFrom(from string, params ...string) (string, error)
func RcptTo(to string, params ...string) (string, error)
func DATA() (string, error)
func RSET() (string, error)
func QUIT() (string, error)
func STARTTLS() (string, error)
func ValidateLine(line string) error
func Wire(reqline string) string
func NewAddress(addr string, params ...string) *Address

// Message — dot-stuffed, terminated DATA payload.
func WriteMessage(src string) string

// Response — Net::SMTP::Response.
func ParseResponse(str string) *Response
func (r *Response) Success() bool
func (r *Response) Continue() bool
func (r *Response) Message() string
func (r *Response) Capabilities() map[string][]string
func (r *Response) CRAMMD5Challenge() ([]byte, error)
func (r *Response) ExceptionClass() ErrorKind

// Errors — keyed by reply code; RubyClass maps to the host exception.
type ErrorKind int
func (k ErrorKind) RubyClass() string
type SMTPError struct { Kind ErrorKind; Response *Response; Msg string }

// Auth — SASL encodings.
func AuthPlain(user, secret string) string
func AuthLoginInitial() string
func AuthLoginUser(user string) string
func AuthLoginSecret(secret string) string
func AuthCramMD5Initial() string
func AuthCramMD5Response(user, secret string, challenge []byte) string

// Session — the convenience driver over a host-supplied Conn.
type Conn interface { /* … */ }
type Session struct { /* … */ }
func NewSession(conn Conn) *Session
```

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at
100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential MRI oracle**: command reqlines, dot-stuffed payloads, reply
parses (status / success? / continue? / exception_class / capabilities), and the
AUTH PLAIN/LOGIN/CRAM-MD5 strings are computed here and compared byte-for-byte
against the system `ruby -rnet/smtp`. The oracle scripts `$stdout.binmode` so
Windows text-mode never pollutes the bytes, and skip themselves where `ruby` is
absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-net-smtp/net-smtp authors.
