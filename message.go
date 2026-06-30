// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import "strings"

// WriteMessage renders a DATA payload exactly as Net::InternetMessageIO#write_message
// puts it on the wire: every line normalised to CRLF, dot-stuffed (a leading "."
// doubled), an unterminated final line CRLF-terminated, an empty source emitted
// as a single CRLF, and the whole closed by the "." + CRLF terminator.
//
//	WriteMessage(".hi\r\n") == "..hi\r\n.\r\n"
//	WriteMessage("")        == "\r\n.\r\n"
func WriteMessage(src string) string {
	var b strings.Builder
	lines, rest := splitCRLFLines(src)
	written := false
	for _, line := range lines {
		b.WriteString(dotStuff(line))
		written = true
	}
	// using_each_crlf_line's tail: flush an unterminated last line, else emit a
	// lone CRLF for an empty source, then always the "." terminator.
	if rest != "" {
		b.WriteString(dotStuff(chompNoArg(rest) + CRLF))
	} else if !written {
		b.WriteString(CRLF)
	}
	b.WriteString("." + CRLF)
	return b.String()
}

// dotStuff mirrors InternetMessageIO#dot_stuff: a line beginning with "." has it
// doubled. Only the leading byte is affected.
func dotStuff(s string) string {
	if strings.HasPrefix(s, ".") {
		return "." + s
	}
	return s
}

// chompNoArg removes a single trailing line ending — "\r\n", "\n", or a lone
// "\r" — mirroring Ruby's String#chomp with no argument, which the tail of
// using_each_crlf_line applies to the unterminated final fragment (@wbuf.chomp).
func chompNoArg(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r") {
		return s[:len(s)-1]
	}
	return s
}

// splitCRLFLines reproduces each_crlf_line's tokeniser, slicing src into complete
// lines (each normalised to a CRLF terminator) and returning any unterminated
// trailing fragment separately. The Ruby regexp is
//
//	/\A[^\r\n]*(?:\n|\r(?:\n|(?!\z)))/
//
// i.e. a run of non-CR/LF bytes followed by: a LF, or a CR+LF, or a CR that is
// not at end-of-string (a bare CR mid-buffer ends a line; a bare CR at the very
// end does not and is left in the fragment). Each match is chomp("\n")+CRLF.
func splitCRLFLines(src string) (lines []string, rest string) {
	i := 0
	n := len(src)
	for i < n {
		// Consume the leading [^\r\n]* run.
		j := i
		for j < n && src[j] != '\r' && src[j] != '\n' {
			j++
		}
		if j == n {
			// No terminator before end of string: unterminated fragment.
			rest = src[i:]
			return lines, rest
		}
		switch src[j] {
		case '\n':
			// "...\n" -> chomp the "\n", append CRLF.
			lines = append(lines, src[i:j]+CRLF)
			i = j + 1
		case '\r':
			if j+1 < n && src[j+1] == '\n' {
				// "<run>\r\n": the match is "<run>\r\n"; Ruby's chomp("\n")
				// strips the whole "\r\n" pair, so line.chomp("\n")+"\r\n" is
				// "<run>"+"\r\n". We append exactly that.
				lines = append(lines, src[i:j]+CRLF)
				i = j + 2
			} else if j+1 < n {
				// Bare CR mid-buffer: the match is "<run>\r"; chomp("\n") is a
				// no-op there, but the yielded form is line.chomp("\n")+"\r\n",
				// and MRI emits "<run>\r\n" (the run plus a normalised CRLF).
				lines = append(lines, src[i:j]+CRLF)
				i = j + 1
			} else {
				// Bare CR at end of string: not matched, stays in fragment.
				rest = src[i:]
				return lines, rest
			}
		}
	}
	return lines, rest
}
