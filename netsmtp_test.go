// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import (
	"reflect"
	"testing"
)

// --- command builders ---------------------------------------------------------

func TestCommandBuilders(t *testing.T) {
	cases := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"helo", func() (string, error) { return HELO("mail.example.com") }, "HELO mail.example.com"},
		{"ehlo", func() (string, error) { return EHLO("mail.example.com") }, "EHLO mail.example.com"},
		{"mailfrom", func() (string, error) { return MailFrom("a@b.com") }, "MAIL FROM:<a@b.com>"},
		{"mailfrom_params", func() (string, error) { return MailFrom("a@b.com", "SIZE=100", "SMTPUTF8") },
			"MAIL FROM:<a@b.com> SIZE=100 SMTPUTF8"},
		{"rcptto", func() (string, error) { return RcptTo("c@d.com") }, "RCPT TO:<c@d.com>"},
		{"rcptto_params", func() (string, error) { return RcptTo("c@d.com", "NOTIFY=SUCCESS") },
			"RCPT TO:<c@d.com> NOTIFY=SUCCESS"},
		{"data", func() (string, error) { return DATA() }, "DATA"},
		{"rset", func() (string, error) { return RSET() }, "RSET"},
		{"quit", func() (string, error) { return QUIT() }, "QUIT"},
		{"starttls", func() (string, error) { return STARTTLS() }, "STARTTLS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.got()
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestValidateLine(t *testing.T) {
	if err := ValidateLine("OK line"); err != nil {
		t.Errorf("valid line rejected: %v", err)
	}
	for _, bad := range []string{"x\ry", "x\ny", "x\r\ny"} {
		if err := ValidateLine(bad); err == nil {
			t.Errorf("bad line %q accepted", bad)
		} else if err.Error() != "A line must not contain CR or LF" {
			t.Errorf("wrong message: %q", err.Error())
		}
	}
}

func TestWire(t *testing.T) {
	line, err := HELO("me")
	if err != nil {
		t.Fatal(err)
	}
	if got := Wire(line); got != "HELO me\r\n" {
		t.Errorf("Wire = %q", got)
	}
}

func TestWriteLineRejects(t *testing.T) {
	if _, err := HELO("dom\nain"); err == nil {
		t.Fatal("HELO accepted CR/LF")
	}
	if _, err := MailFrom("a\nb"); err == nil {
		t.Fatal("MailFrom accepted CR/LF")
	}
	if _, err := RcptTo("a\nb"); err == nil {
		t.Fatal("RcptTo accepted CR/LF")
	}
}

func TestNewAddressDedup(t *testing.T) {
	a := NewAddress("a@b", "X=1", "Y=2", "X=1")
	if !reflect.DeepEqual(a.Parameters, []string{"X=1", "Y=2"}) {
		t.Errorf("dedup failed: %v", a.Parameters)
	}
	if a.Address != "a@b" {
		t.Errorf("address: %q", a.Address)
	}
}

// --- response parsing ---------------------------------------------------------

func TestParseResponse(t *testing.T) {
	r := ParseResponse("250 OK\n")
	if r.Status != "250" || r.String != "250 OK\n" {
		t.Fatalf("parse: %+v", r)
	}
	if !r.Success() || r.Continue() {
		t.Errorf("250 flags wrong: success=%v continue=%v", r.Success(), r.Continue())
	}
	if c := ParseResponse("354 go\n"); !c.Continue() || c.Success() {
		t.Errorf("354 flags wrong")
	}
}

func TestParseResponseShort(t *testing.T) {
	// str[0,3] on a string shorter than three bytes returns what exists.
	r := ParseResponse("25")
	if r.Status != "25" {
		t.Errorf("short status: %q", r.Status)
	}
	if r.statusTypeChar() != "2" {
		t.Errorf("statusTypeChar: %q", r.statusTypeChar())
	}
	// Empty status -> empty type char (covers the len==0 branch).
	e := ParseResponse("")
	if e.statusTypeChar() != "" {
		t.Errorf("empty type char: %q", e.statusTypeChar())
	}
	if e.Success() || e.Continue() {
		t.Errorf("empty should be neither success nor continue")
	}
}

func TestMessage(t *testing.T) {
	r := ParseResponse("250-a\n250 b\n")
	if got := r.Message(); got != "250-a\n" {
		t.Errorf("message multiline: %q", got)
	}
	// No newline at all -> whole string.
	if got := ParseResponse("250 z").Message(); got != "250 z" {
		t.Errorf("message single: %q", got)
	}
}

func TestExceptionClass(t *testing.T) {
	cases := []struct {
		status string
		kind   ErrorKind
		ruby   string
	}{
		{"421", KindServerBusy, "Net::SMTPServerBusy"},
		{"450", KindServerBusy, "Net::SMTPServerBusy"},
		{"500", KindSyntax, "Net::SMTPSyntaxError"},
		{"502", KindSyntax, "Net::SMTPSyntaxError"},
		{"530", KindAuthentication, "Net::SMTPAuthenticationError"},
		{"535", KindAuthentication, "Net::SMTPAuthenticationError"},
		{"550", KindFatal, "Net::SMTPFatalError"},
		{"554", KindFatal, "Net::SMTPFatalError"},
		{"100", KindUnknown, "Net::SMTPUnknownError"},
		{"250", KindUnknown, "Net::SMTPUnknownError"},
		{"600", KindUnknown, "Net::SMTPUnknownError"},
	}
	for _, c := range cases {
		k := ParseResponse(c.status + " x\n").ExceptionClass()
		if k != c.kind {
			t.Errorf("%s -> %v want %v", c.status, k, c.kind)
		}
		if k.RubyClass() != c.ruby || k.String() != c.ruby {
			t.Errorf("%s -> ruby %q want %q", c.status, k.RubyClass(), c.ruby)
		}
	}
}

func TestCapabilities(t *testing.T) {
	r := ParseResponse("250-PIPELINING\n250-SIZE 10240000\n250-AUTH PLAIN LOGIN\n250 HELP\n")
	caps := r.Capabilities()
	want := map[string][]string{
		"SIZE": {"10240000"},
		"AUTH": {"PLAIN", "LOGIN"},
		"HELP": {},
	}
	if !reflect.DeepEqual(caps, want) {
		t.Errorf("caps = %v want %v", caps, want)
	}
	// Single-line reply -> empty map (String[3,1] != "-").
	if c := ParseResponse("250 OK\n").Capabilities(); len(c) != 0 {
		t.Errorf("single-line caps: %v", c)
	}
	// Too short to index [3] -> empty map.
	if c := ParseResponse("25\n").Capabilities(); len(c) != 0 {
		t.Errorf("short caps: %v", c)
	}
	// The first line is always dropped (Ruby's lines.drop(1)); a duplicate verb on
	// later lines keeps the last line's arguments (Ruby hash assignment).
	dup := ParseResponse("250-greeting\n250-AUTH PLAIN\n250 AUTH LOGIN\n").Capabilities()
	if !reflect.DeepEqual(dup, map[string][]string{"AUTH": {"LOGIN"}}) {
		t.Errorf("dup caps: %v", dup)
	}
	// A continuation line with empty body after the prefix is skipped here (MRI
	// keys it under nil, which has no representation in a string map; degenerate,
	// never seen on the wire).
	if c := ParseResponse("250-greeting\n250 \n").Capabilities(); len(c) != 0 {
		t.Errorf("empty-body caps: %v", c)
	}
	// A continuation line of fewer than 4 bytes (body stays "") is skipped.
	if c := ParseResponse("250-greeting\n25\n").Capabilities(); len(c) != 0 {
		t.Errorf("short-line caps: %v", c)
	}
}

// --- dot-stuffing -------------------------------------------------------------

func TestWriteMessage(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"simple", "Hello\r\nWorld\r\n", "Hello\r\nWorld\r\n.\r\n"},
		{"leadingdot", ".hidden\r\n", "..hidden\r\n.\r\n"},
		{"dotmid", "a\r\n.b\r\nc\r\n", "a\r\n..b\r\nc\r\n.\r\n"},
		{"lf_only", "line1\nline2\n", "line1\r\nline2\r\n.\r\n"},
		{"bare_cr", "x\ry\r", "x\r\ny\r\n.\r\n"},
		{"no_trailing", "abc", "abc\r\n.\r\n"},
		{"empty", "", "\r\n.\r\n"},
		{"justdot", ".\r\n", "..\r\n.\r\n"},
		{"dotdot", "..\r\n", "...\r\n.\r\n"},
		{"bare_cr_end", "x\ry", "x\r\ny\r\n.\r\n"},
		{"dot_unterminated", ".end", "..end\r\n.\r\n"},
		{"crlf_then_dot_end", "a\r\n.", "a\r\n..\r\n.\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WriteMessage(c.in); got != c.want {
				t.Errorf("WriteMessage(%q) = %q want %q", c.in, got, c.want)
			}
		})
	}
}

func TestChompNoArg(t *testing.T) {
	for in, want := range map[string]string{"x\r\n": "x", "x\n": "x", "x\r": "x", "x": "x"} {
		if got := chompNoArg(in); got != want {
			t.Errorf("chompNoArg(%q) = %q want %q", in, got, want)
		}
	}
}

// --- auth ---------------------------------------------------------------------

func TestAuthPlain(t *testing.T) {
	if got := AuthPlain("user", "pass"); got != "AUTH PLAIN AHVzZXIAcGFzcw==" {
		t.Errorf("plain = %q", got)
	}
}

func TestAuthLogin(t *testing.T) {
	if AuthLoginInitial() != "AUTH LOGIN" {
		t.Error("login initial")
	}
	if got := AuthLoginUser("user"); got != "dXNlcg==" {
		t.Errorf("login user = %q", got)
	}
	if got := AuthLoginSecret("pass"); got != "cGFzcw==" {
		t.Errorf("login secret = %q", got)
	}
}

func TestAuthCramMD5(t *testing.T) {
	if AuthCramMD5Initial() != "AUTH CRAM-MD5" {
		t.Error("cram initial")
	}
	// RFC 2195 canonical vector.
	chal := []byte("<1896.697170952@postoffice.example.net>")
	got := AuthCramMD5Response("tim", "tanstaaftanstaaf", chal)
	// "tim 3dbc88f0624776a737b39093f6eb6427" base64.
	want := base64M([]byte("tim 3dbc88f0624776a737b39093f6eb6427"))
	if got != want {
		t.Errorf("cram = %q want %q", got, want)
	}
	if cramMD5Response([]byte("tanstaaftanstaaf"), chal) != "3dbc88f0624776a737b39093f6eb6427" {
		t.Error("cram hex mismatch")
	}
}

func TestCramSecretLongPath(t *testing.T) {
	// Secret > 64 bytes is pre-hashed; just exercise the branch and stability.
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'x'
	}
	a := cramMD5Response(long, []byte("c"))
	b := cramMD5Response(long, []byte("c"))
	if a != b || len(a) != 32 {
		t.Errorf("long-secret cram unstable: %q %q", a, b)
	}
}

func TestDecodeBase64M(t *testing.T) {
	// Decodes standard base64, ignores stray non-alphabet bytes, tolerates the
	// missing/explicit padding the 'm' unpack allows.
	in := base64M([]byte("hello world"))
	got, err := decodeBase64M(in)
	if err != nil || string(got) != "hello world" {
		t.Fatalf("decode = %q err %v", got, err)
	}
	// With a newline injected (servers wrap) and padding present.
	got, err = decodeBase64M("aGVsbG8=\n")
	if err != nil || string(got) != "hello" {
		t.Fatalf("decode padded = %q err %v", got, err)
	}
	// Empty input.
	got, err = decodeBase64M("")
	if err != nil || len(got) != 0 {
		t.Fatalf("decode empty = %q err %v", got, err)
	}
	// Invalid (lone char) -> error.
	if _, err := decodeBase64M("A"); err == nil {
		t.Error("expected decode error for lone char")
	}
}

func TestResponseCRAMMD5Challenge(t *testing.T) {
	// A "334 <b64>" reply: split on space, decode field 1.
	enc := base64M([]byte("<challenge@host>"))
	r := ParseResponse("334 " + enc + "\n")
	got, err := r.CRAMMD5Challenge()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// The trailing "\n" is part of field 1 here; decodeBase64M ignores it.
	if string(got) != "<challenge@host>" {
		t.Errorf("challenge = %q", got)
	}
	// A reply with no space -> empty decode (split has one field).
	r2 := ParseResponse("334\n")
	if g, err := r2.CRAMMD5Challenge(); err != nil || len(g) != 0 {
		t.Errorf("no-space challenge = %q err %v", g, err)
	}
}
