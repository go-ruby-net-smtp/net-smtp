// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import "strings"

// Response is a server reply, mirroring Net::SMTP::Response. It holds the raw
// three-digit Status and the full reply String (every line, terminated by a
// bare "\n"), exactly as MRI's recv_response assembles it: each wire line has
// its trailing CRLF chopped and a single "\n" re-appended.
//
//	res := ParseResponse("250-PIPELINING\n250 HELP\n")
//	res.Status   // "250"
//	res.Success() // true
type Response struct {
	// Status is the first three bytes of the reply (the numeric code). MRI does
	// not validate that they are digits — it is literally str[0,3].
	Status string
	// String is the human-readable reply text: the whole multi-line reply, each
	// line CRLF-chopped and "\n"-terminated.
	String string
}

// ParseResponse mirrors Net::SMTP::Response.parse(str): the status is the first
// three bytes (str[0,3]) and the string is the whole reply verbatim. A string
// shorter than three bytes yields a status equal to whatever bytes exist, just
// as Ruby's str[0,3] returns the available prefix.
func ParseResponse(str string) *Response {
	n := 3
	if len(str) < n {
		n = len(str)
	}
	return &Response{Status: str[:n], String: str}
}

// statusTypeChar is the first byte of the status, or "" when the status is
// empty. Net::SMTP::Response#status_type_char is @status[0,1].
func (r *Response) statusTypeChar() string {
	if len(r.Status) == 0 {
		return ""
	}
	return r.Status[:1]
}

// Success reports a 2xx Positive Completion reply (Net::SMTP::Response#success?).
func (r *Response) Success() bool { return r.statusTypeChar() == "2" }

// Continue reports a 3xx Positive Intermediate reply (Response#continue?).
func (r *Response) Continue() bool { return r.statusTypeChar() == "3" }

// Message is the first line of the reply text (Response#message): String.lines.first.
// When String is empty MRI returns nil; here that is the empty string.
func (r *Response) Message() string {
	if i := strings.IndexByte(r.String, '\n'); i >= 0 {
		return r.String[:i+1]
	}
	return r.String
}

// CRAMMD5Challenge decodes the base64 CRAM-MD5 challenge carried in the reply,
// mirroring Response#cram_md5_challenge: @string.split(/ /)[1].unpack1('m'). The
// reply is split on single spaces and the second field base64-decoded.
func (r *Response) CRAMMD5Challenge() ([]byte, error) {
	parts := strings.Split(r.String, " ")
	if len(parts) < 2 {
		return decodeBase64M("")
	}
	return decodeBase64M(parts[1])
}

// Capabilities parses the EHLO capability map, mirroring Response#capabilities.
// It returns an empty map unless the reply is multi-line (String[3,1] == "-");
// then each continuation line (drop the first) is split on spaces after its
// "NNN-" / "NNN " prefix, keying the verb to its arguments. A duplicate verb
// keeps the last line's arguments, matching Ruby's hash assignment.
func (r *Response) Capabilities() map[string][]string {
	h := map[string][]string{}
	if len(r.String) < 4 || r.String[3] != '-' {
		return h
	}
	lines := splitLines(r.String)
	for _, line := range lines[1:] {
		body := ""
		if len(line) > 4 {
			body = line[4:]
		}
		fields := strings.Fields(body)
		if len(fields) == 0 {
			continue
		}
		// fields is non-nil from strings.Fields, so fields[1:] is a non-nil empty
		// slice for a lone verb — matching Ruby's `k, *v` giving v == [].
		h[fields[0]] = fields[1:]
	}
	return h
}

// splitLines splits on "\n" like Ruby's String#lines but without keeping the
// terminators and without a trailing empty element (String always ends "\n").
func splitLines(s string) []string {
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i])
		s = s[i+1:]
	}
	return out
}

// ExceptionClass selects the error type for a non-success reply, mirroring
// Response#exception_class's case on @status: /\A4/→ServerBusy, /\A50/→Syntax,
// /\A53/→Authentication, /\A5/→Fatal, else→Unknown. The ordering matters: 50x is
// tested before the general 5xx, and 53x before it too (53 matches /\A50/? no —
// 53 is tested by /\A53/ which comes after /\A50/, and 530 does not start "50").
func (r *Response) ExceptionClass() ErrorKind {
	s := r.Status
	switch {
	case strings.HasPrefix(s, "4"):
		return KindServerBusy
	case strings.HasPrefix(s, "50"):
		return KindSyntax
	case strings.HasPrefix(s, "53"):
		return KindAuthentication
	case strings.HasPrefix(s, "5"):
		return KindFatal
	default:
		return KindUnknown
	}
}
