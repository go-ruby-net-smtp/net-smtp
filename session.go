// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

// Conn is the transport seam the host supplies. It abstracts the TCP/TLS socket
// Net::SMTP holds in @socket; the codec drives it but never creates it. WriteLine
// sends a request line (the impl appends CRLF, or accepts the pre-terminated form
// from the command builders — implementations append CRLF themselves to mirror
// BufferedIO#writeline). ReadLine returns one reply line with its CRLF removed
// (Net::BufferedIO#readline). WriteRaw writes a DATA payload verbatim. StartTLS
// upgrades the connection in place.
type Conn interface {
	// WriteLine sends a request line; the implementation appends CRLF.
	WriteLine(line string) error
	// ReadLine reads one reply line and returns it with the trailing CRLF
	// removed (Ruby readline = readuntil("\n").chop).
	ReadLine() (string, error)
	// WriteRaw writes the given bytes to the connection unchanged (used for the
	// already dot-stuffed, terminated DATA payload).
	WriteRaw(b string) error
	// StartTLS upgrades the connection to TLS in place.
	StartTLS() error
}

// Session drives the codec against a Conn, sequencing commands and replies the
// way Net::SMTP does: validate_line, writeline, recv_response (multi-line aware),
// then check_response. It is a thin convenience over the pure functions; a host
// that wants finer control can call those directly.
type Session struct {
	conn Conn
	caps map[string][]string
}

// NewSession wraps a Conn.
func NewSession(conn Conn) *Session { return &Session{conn: conn} }

// RecvResponse reads a full reply, mirroring Net::SMTP#recv_response: it reads
// lines until one whose 4th byte is not "-", assembling them (CRLF-stripped, each
// re-terminated with "\n") into the Response string, then parses it.
func (s *Session) RecvResponse() (*Response, error) {
	var buf []byte
	for {
		line, err := s.conn.ReadLine()
		if err != nil {
			return nil, err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
		if len(line) < 4 || line[3] != '-' {
			break
		}
	}
	return ParseResponse(string(buf)), nil
}

// getOK sends a request line and reads the reply, raising on a non-success reply,
// mirroring Net::SMTP#getok (validate_line, writeline, recv_response, check_response).
func (s *Session) getOK(line string, err error) (*Response, error) {
	if err != nil {
		return nil, err
	}
	if e := s.conn.WriteLine(line); e != nil {
		return nil, e
	}
	res, e := s.RecvResponse()
	if e != nil {
		return nil, e
	}
	if !res.Success() {
		return res, errorFor(res)
	}
	return res, nil
}

// getResponse sends a request line and reads the reply WITHOUT the success check,
// mirroring Net::SMTP#get_response (used by DATA and the authenticators).
func (s *Session) getResponse(line string, err error) (*Response, error) {
	if err != nil {
		return nil, err
	}
	if e := s.conn.WriteLine(line); e != nil {
		return nil, e
	}
	return s.RecvResponse()
}

// Helo sends HELO and returns the reply (Net::SMTP#helo).
func (s *Session) Helo(domain string) (*Response, error) { return s.getOK(HELO(domain)) }

// Ehlo sends EHLO, returns the reply, and caches its capabilities for later
// AUTH/SMTPUTF8 decisions (Net::SMTP#ehlo).
func (s *Session) Ehlo(domain string) (*Response, error) {
	res, err := s.getOK(EHLO(domain))
	if err == nil {
		s.caps = res.Capabilities()
	}
	return res, err
}

// Capabilities returns the capability map from the last successful Ehlo, or nil.
func (s *Session) Capabilities() map[string][]string { return s.caps }

// MailFrom sends MAIL FROM with optional ESMTP parameters (Net::SMTP#mailfrom).
func (s *Session) MailFrom(from string, params ...string) (*Response, error) {
	return s.getOK(MailFrom(from, params...))
}

// RcptTo sends RCPT TO with optional ESMTP parameters (Net::SMTP#rcptto).
func (s *Session) RcptTo(to string, params ...string) (*Response, error) {
	return s.getOK(RcptTo(to, params...))
}

// Rset sends RSET (Net::SMTP#rset).
func (s *Session) Rset() (*Response, error) { return s.getOK(RSET()) }

// Quit sends QUIT (Net::SMTP#quit).
func (s *Session) Quit() (*Response, error) { return s.getOK(QUIT()) }

// Data performs the DATA exchange (Net::SMTP#data): send DATA, require a 3xx
// continue reply (else SMTPUnknownError "could not get 3xx"), write the
// dot-stuffed message, then read and check the final reply.
func (s *Session) Data(msg string) (*Response, error) {
	res, err := s.getResponse(DATA())
	if err != nil {
		return nil, err
	}
	if !res.Continue() {
		return res, continueError(res)
	}
	if e := s.conn.WriteRaw(WriteMessage(msg)); e != nil {
		return nil, e
	}
	final, e := s.RecvResponse()
	if e != nil {
		return nil, e
	}
	if !final.Success() {
		return final, errorFor(final)
	}
	return final, nil
}

// StartTLS sends STARTTLS, checks the reply, then upgrades the connection
// (Net::SMTP#starttls returns the getok reply; the TLS handshake is the seam).
func (s *Session) StartTLS() (*Response, error) {
	res, err := s.getOK(STARTTLS())
	if err != nil {
		return res, err
	}
	if e := s.conn.StartTLS(); e != nil {
		return res, e
	}
	return res, nil
}

// AuthPlain runs the AUTH PLAIN exchange (AuthPlain#auth): one line, success
// required.
func (s *Session) AuthPlain(user, secret string) (*Response, error) {
	return s.finish(AuthPlain(user, secret))
}

// AuthLogin runs the AUTH LOGIN exchange (AuthLogin#auth): the initial line and
// the user name each require a 3xx continue, then the secret requires success.
func (s *Session) AuthLogin(user, secret string) (*Response, error) {
	if _, err := s.continueAuth(AuthLoginInitial()); err != nil {
		return nil, err
	}
	if _, err := s.continueAuth(AuthLoginUser(user)); err != nil {
		return nil, err
	}
	return s.finish(AuthLoginSecret(secret))
}

// AuthCramMD5 runs the AUTH CRAM-MD5 exchange (AuthCramMD5#auth): the initial
// line yields the challenge (3xx continue), whose payload is decoded and answered.
func (s *Session) AuthCramMD5(user, secret string) (*Response, error) {
	challB64, err := s.continueAuth(AuthCramMD5Initial())
	if err != nil {
		return nil, err
	}
	challenge, err := decodeBase64M(challB64)
	if err != nil {
		return nil, err
	}
	return s.finish(AuthCramMD5Response(user, secret, challenge))
}

// continueAuth mirrors Authenticator#continue: send the line, require a 3xx reply
// (else raise its exception_class), and return the reply's second space-separated
// field (the base64 challenge for CRAM-MD5; the prompt otherwise).
func (s *Session) continueAuth(line string) (string, error) {
	res, err := s.getResponse(line, nil)
	if err != nil {
		return "", err
	}
	if !res.Continue() {
		return "", errorFor(res)
	}
	fields := fieldsSpace(res.String)
	if len(fields) < 2 {
		return "", nil
	}
	return fields[1], nil
}

// finish mirrors Authenticator#finish: send the line and require a success reply
// (else raise SMTPAuthenticationError).
func (s *Session) finish(line string) (*Response, error) {
	res, err := s.getResponse(line, nil)
	if err != nil {
		return nil, err
	}
	if !res.Success() {
		return res, &SMTPError{Kind: KindAuthentication, Response: res}
	}
	return res, nil
}

// fieldsSpace splits on runs of whitespace like Ruby String#split with no arg,
// dropping leading whitespace and empty fields. Authenticator#continue uses
// res.string.split[1].
func fieldsSpace(s string) []string {
	var out []string
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		ws := c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
		if ws {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}
