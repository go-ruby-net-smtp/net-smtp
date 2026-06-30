// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeConn is a scripted transport: it replays canned reply lines and records the
// request lines and raw payloads written, so the Session driver runs against a
// deterministic seam (no real socket).
type fakeConn struct {
	replies   []string // reply lines, CRLF already stripped (as ReadLine returns)
	ri        int
	written   []string
	raw       []string
	tlsCalled bool
	writeErr  error
	writeFail int // fail the Nth (1-based) WriteLine call; 0 = never
	wcalls    int
	readErr   error
	rawErr    error
	tlsErr    error
}

func (c *fakeConn) WriteLine(line string) error {
	c.wcalls++
	if c.writeErr != nil {
		return c.writeErr
	}
	if c.writeFail != 0 && c.wcalls == c.writeFail {
		return errors.New("write failed")
	}
	c.written = append(c.written, line)
	return nil
}

func (c *fakeConn) ReadLine() (string, error) {
	if c.readErr != nil {
		return "", c.readErr
	}
	if c.ri >= len(c.replies) {
		return "", errors.New("no more replies")
	}
	r := c.replies[c.ri]
	c.ri++
	return r, nil
}

func (c *fakeConn) WriteRaw(b string) error {
	if c.rawErr != nil {
		return c.rawErr
	}
	c.raw = append(c.raw, b)
	return nil
}

func (c *fakeConn) StartTLS() error {
	if c.tlsErr != nil {
		return c.tlsErr
	}
	c.tlsCalled = true
	return nil
}

func TestSessionHeloEhlo(t *testing.T) {
	c := &fakeConn{replies: []string{"250 mail.example.com", "250-mail.example.com", "250-SIZE 100", "250 AUTH PLAIN"}}
	s := NewSession(c)
	if _, err := s.Helo("me"); err != nil {
		t.Fatalf("helo: %v", err)
	}
	res, err := s.Ehlo("me")
	if err != nil {
		t.Fatalf("ehlo: %v", err)
	}
	if !res.Success() {
		t.Error("ehlo not success")
	}
	want := map[string][]string{"SIZE": {"100"}, "AUTH": {"PLAIN"}}
	if !reflect.DeepEqual(s.Capabilities(), want) {
		t.Errorf("caps = %v", s.Capabilities())
	}
	if c.written[0] != "HELO me" || c.written[1] != "EHLO me" {
		t.Errorf("written = %v", c.written)
	}
}

func TestSessionEhloError(t *testing.T) {
	c := &fakeConn{replies: []string{"550 no"}}
	s := NewSession(c)
	res, err := s.Ehlo("me")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != KindFatal {
		t.Errorf("wrong err: %v", err)
	}
	if res == nil || s.Capabilities() != nil {
		t.Error("caps should be unset on error")
	}
}

func TestSessionMailRcptRsetQuit(t *testing.T) {
	c := &fakeConn{replies: []string{"250 ok", "250 ok", "250 ok", "221 bye"}}
	s := NewSession(c)
	if _, err := s.MailFrom("a@b", "SIZE=1"); err != nil {
		t.Fatalf("mail: %v", err)
	}
	if _, err := s.RcptTo("c@d"); err != nil {
		t.Fatalf("rcpt: %v", err)
	}
	if _, err := s.Rset(); err != nil {
		t.Fatalf("rset: %v", err)
	}
	if _, err := s.Quit(); err != nil {
		t.Fatalf("quit: %v", err)
	}
	if c.written[0] != "MAIL FROM:<a@b> SIZE=1" || c.written[1] != "RCPT TO:<c@d>" {
		t.Errorf("written = %v", c.written)
	}
}

func TestSessionData(t *testing.T) {
	c := &fakeConn{replies: []string{"354 go ahead", "250 queued"}}
	s := NewSession(c)
	res, err := s.Data("Subject: hi\r\n.secret\r\n")
	if err != nil {
		t.Fatalf("data: %v", err)
	}
	if !res.Success() {
		t.Error("data not success")
	}
	if len(c.raw) != 1 || c.raw[0] != "Subject: hi\r\n..secret\r\n.\r\n" {
		t.Errorf("raw = %q", c.raw)
	}
}

func TestSessionDataNoContinue(t *testing.T) {
	c := &fakeConn{replies: []string{"451 busy"}}
	s := NewSession(c)
	res, err := s.Data("x")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != KindUnknown {
		t.Errorf("wrong kind: %v", err)
	}
	if !strings.Contains(se.Error(), "could not get 3xx") {
		t.Errorf("msg = %q", se.Error())
	}
	if res == nil {
		t.Error("res should be returned")
	}
}

func TestSessionDataFinalError(t *testing.T) {
	c := &fakeConn{replies: []string{"354 ok", "550 rejected"}}
	s := NewSession(c)
	_, err := s.Data("x")
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != KindFatal {
		t.Errorf("wrong err: %v", err)
	}
}

func TestSessionStartTLS(t *testing.T) {
	c := &fakeConn{replies: []string{"220 ready"}}
	s := NewSession(c)
	if _, err := s.StartTLS(); err != nil {
		t.Fatalf("starttls: %v", err)
	}
	if !c.tlsCalled {
		t.Error("TLS upgrade not invoked")
	}
}

func TestSessionStartTLSReplyError(t *testing.T) {
	c := &fakeConn{replies: []string{"454 no tls"}}
	s := NewSession(c)
	if _, err := s.StartTLS(); err == nil {
		t.Fatal("expected error")
	}
	if c.tlsCalled {
		t.Error("TLS upgraded despite error reply")
	}
}

func TestSessionStartTLSUpgradeError(t *testing.T) {
	c := &fakeConn{replies: []string{"220 ready"}, tlsErr: errors.New("handshake")}
	s := NewSession(c)
	if _, err := s.StartTLS(); err == nil {
		t.Fatal("expected upgrade error")
	}
}

func TestSessionAuthPlain(t *testing.T) {
	c := &fakeConn{replies: []string{"235 ok"}}
	s := NewSession(c)
	if _, err := s.AuthPlain("user", "pass"); err != nil {
		t.Fatalf("auth: %v", err)
	}
	if c.written[0] != "AUTH PLAIN AHVzZXIAcGFzcw==" {
		t.Errorf("written = %v", c.written)
	}
}

func TestSessionAuthPlainFail(t *testing.T) {
	c := &fakeConn{replies: []string{"535 bad"}}
	s := NewSession(c)
	_, err := s.AuthPlain("u", "p")
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != KindAuthentication {
		t.Errorf("wrong err: %v", err)
	}
}

func TestSessionAuthLogin(t *testing.T) {
	c := &fakeConn{replies: []string{"334 VXNlcm5hbWU6", "334 UGFzc3dvcmQ6", "235 ok"}}
	s := NewSession(c)
	if _, err := s.AuthLogin("user", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !reflect.DeepEqual(c.written, []string{"AUTH LOGIN", "dXNlcg==", "cGFzcw=="}) {
		t.Errorf("written = %v", c.written)
	}
}

func TestSessionAuthLoginInitialFail(t *testing.T) {
	c := &fakeConn{replies: []string{"500 no"}}
	s := NewSession(c)
	if _, err := s.AuthLogin("u", "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionAuthLoginUserFail(t *testing.T) {
	c := &fakeConn{replies: []string{"334 prompt", "535 bad user"}}
	s := NewSession(c)
	_, err := s.AuthLogin("u", "p")
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != KindAuthentication {
		t.Errorf("wrong err: %v", err)
	}
}

func TestSessionAuthCramMD5(t *testing.T) {
	chal := base64M([]byte("<1896.697170952@postoffice.example.net>"))
	c := &fakeConn{replies: []string{"334 " + chal, "235 ok"}}
	s := NewSession(c)
	if _, err := s.AuthCramMD5("tim", "tanstaaftanstaaf"); err != nil {
		t.Fatalf("cram: %v", err)
	}
	if c.written[0] != "AUTH CRAM-MD5" {
		t.Errorf("first = %q", c.written[0])
	}
	want := base64M([]byte("tim 3dbc88f0624776a737b39093f6eb6427"))
	if c.written[1] != want {
		t.Errorf("response = %q want %q", c.written[1], want)
	}
}

func TestSessionAuthCramMD5InitialFail(t *testing.T) {
	c := &fakeConn{replies: []string{"500 no"}}
	s := NewSession(c)
	if _, err := s.AuthCramMD5("u", "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionAuthCramMD5BadChallenge(t *testing.T) {
	// A continue reply whose challenge field is undecodable -> decode error.
	c := &fakeConn{replies: []string{"334 A"}}
	s := NewSession(c)
	if _, err := s.AuthCramMD5("u", "p"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestSessionContinueNoField(t *testing.T) {
	// A 334 reply with no second field -> continueAuth returns "" challenge,
	// which decodes to empty and proceeds to finish.
	c := &fakeConn{replies: []string{"334", "235 ok"}}
	s := NewSession(c)
	if _, err := s.AuthCramMD5("u", "p"); err != nil {
		t.Fatalf("cram empty challenge: %v", err)
	}
}

// --- IO-error propagation -----------------------------------------------------

func TestSessionWriteErr(t *testing.T) {
	c := &fakeConn{writeErr: errors.New("boom")}
	s := NewSession(c)
	if _, err := s.Helo("x"); err == nil {
		t.Fatal("expected write error")
	}
	if _, err := s.getResponse(DATA()); err == nil {
		t.Fatal("expected write error in getResponse")
	}
}

func TestSessionReadErr(t *testing.T) {
	c := &fakeConn{readErr: errors.New("boom")}
	s := NewSession(c)
	if _, err := s.RecvResponse(); err == nil {
		t.Fatal("expected read error")
	}
	if _, err := s.Helo("x"); err == nil {
		t.Fatal("expected read error via getOK")
	}
}

func TestSessionRawErr(t *testing.T) {
	c := &fakeConn{replies: []string{"354 go"}, rawErr: errors.New("boom")}
	s := NewSession(c)
	if _, err := s.Data("x"); err == nil {
		t.Fatal("expected raw-write error")
	}
}

func TestSessionDataFinalReadErr(t *testing.T) {
	// Continue OK and raw write OK, but the final recv fails.
	c := &fakeConn{replies: []string{"354 go"}}
	s := NewSession(c)
	if _, err := s.Data("x"); err == nil {
		t.Fatal("expected final read error")
	}
}

func TestSessionBuilderErrPropagates(t *testing.T) {
	// getOK / getResponse receive a builder error (bad domain) and pass it on.
	c := &fakeConn{}
	s := NewSession(c)
	if _, err := s.Helo("a\nb"); err == nil {
		t.Fatal("expected builder error")
	}
	if _, err := s.MailFrom("a\nb"); err == nil {
		t.Fatal("expected builder error")
	}
}

func TestGetResponseBuilderErr(t *testing.T) {
	// get_response calls validate_line first; a builder error (bad reqline)
	// short-circuits before any write. This is the path DATA would take if its
	// reqline ever contained a bare CR/LF.
	s := NewSession(&fakeConn{})
	if _, err := s.getResponse(HELO("a\nb")); err == nil {
		t.Fatal("expected builder error from getResponse")
	}
}

func TestSessionDataWriteErr(t *testing.T) {
	// The DATA command's WriteLine (1st write) fails -> getResponse returns it.
	c := &fakeConn{writeFail: 1}
	s := NewSession(c)
	if _, err := s.Data("x"); err == nil {
		t.Fatal("expected DATA write error")
	}
}

func TestSessionContinueAuthWriteErr(t *testing.T) {
	// continueAuth's getResponse write fails.
	c := &fakeConn{writeFail: 1}
	s := NewSession(c)
	if _, err := s.AuthCramMD5("u", "p"); err == nil {
		t.Fatal("expected continueAuth write error")
	}
}

func TestSessionFinishWriteErr(t *testing.T) {
	// finish's getResponse write fails (AUTH PLAIN is a single finish call).
	c := &fakeConn{writeFail: 1}
	s := NewSession(c)
	if _, err := s.AuthPlain("u", "p"); err == nil {
		t.Fatal("expected finish write error")
	}
}

func TestSplitLinesNoTrailingNewline(t *testing.T) {
	// A string with no trailing "\n" hits splitLines' final-fragment branch; it
	// reaches Capabilities via a malformed multi-line reply.
	got := splitLines("a\nb")
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("splitLines = %v", got)
	}
}

func TestFieldsSpaceTrailingField(t *testing.T) {
	// A field that runs to end-of-string (no trailing whitespace) is emitted.
	if got := fieldsSpace("a b"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("fieldsSpace = %v", got)
	}
}

func TestErrorDefaults(t *testing.T) {
	// SMTPError with explicit message.
	e := &SMTPError{Kind: KindFatal, Msg: "custom"}
	if e.Error() != "custom" {
		t.Errorf("explicit msg: %q", e.Error())
	}
	// Default message = response first line.
	e2 := &SMTPError{Kind: KindFatal, Response: ParseResponse("550 oops\n")}
	if e2.Error() != "550 oops\n" {
		t.Errorf("default msg: %q", e2.Error())
	}
	// No message and no response -> the ruby class name.
	e3 := &SMTPError{Kind: KindUnknown}
	if e3.Error() != "Net::SMTPUnknownError" {
		t.Errorf("fallback msg: %q", e3.Error())
	}
}

func TestFieldsSpace(t *testing.T) {
	if got := fieldsSpace("  a\tb \n c "); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("fieldsSpace = %v", got)
	}
	if got := fieldsSpace("   "); got != nil {
		t.Errorf("all-ws = %v", got)
	}
}
