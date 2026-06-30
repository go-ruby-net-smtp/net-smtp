// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import "strings"

// CRLF is the wire line terminator (Net writeline appends "\r\n").
const CRLF = "\r\n"

// ErrBareCRLF is returned by ValidateLine and the command builders when a request
// line contains a bare CR or LF, mirroring Net::SMTP#validate_line which raises
// ArgumentError, "A line must not contain CR or LF".
type ErrBareCRLF struct{}

func (ErrBareCRLF) Error() string { return "A line must not contain CR or LF" }

// ValidateLine mirrors Net::SMTP#validate_line: a request line must not contain
// a bare CR or LF (RFC 5321). Any "\r" or "\n" is rejected.
func ValidateLine(line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return ErrBareCRLF{}
	}
	return nil
}

// Wire renders a request line as it goes on the wire: the line followed by CRLF
// (Net::BufferedIO#writeline appends "\r\n" to the reqline). It does not validate;
// the command builders already did.
func Wire(reqline string) string { return reqline + CRLF }

// Address mirrors Net::SMTP::Address: an envelope address plus its ESMTP
// parameters. Parameters are deduplicated preserving first-seen order, each
// rendered "k=v" (or bare "k") just as the Ruby constructor does.
type Address struct {
	Address    string
	Parameters []string
}

// NewAddress builds an Address from a mail address and zero or more "k=v" (or
// bare) parameter strings, deduplicating while preserving order (Ruby's #uniq).
func NewAddress(addr string, params ...string) *Address {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range params {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return &Address{Address: addr, Parameters: out}
}

// HELO builds the "HELO <domain>" request line (Net::SMTP#helo).
func HELO(domain string) (string, error) { return command("HELO " + domain) }

// EHLO builds the "EHLO <domain>" request line (Net::SMTP#ehlo).
func EHLO(domain string) (string, error) { return command("EHLO " + domain) }

// MailFrom builds the "MAIL FROM:<addr>" request line plus any ESMTP parameters,
// mirroring Net::SMTP#mailfrom: (["MAIL FROM:<addr>"] + params).join(' ').
func MailFrom(from string, params ...string) (string, error) {
	a := NewAddress(from, params...)
	parts := append([]string{"MAIL FROM:<" + a.Address + ">"}, a.Parameters...)
	return command(strings.Join(parts, " "))
}

// RcptTo builds the "RCPT TO:<addr>" request line plus any ESMTP parameters,
// mirroring Net::SMTP#rcptto: (["RCPT TO:<addr>"] + params).join(' ').
func RcptTo(to string, params ...string) (string, error) {
	a := NewAddress(to, params...)
	parts := append([]string{"RCPT TO:<" + a.Address + ">"}, a.Parameters...)
	return command(strings.Join(parts, " "))
}

// DATA builds the "DATA" request line (Net::SMTP#data sends get_response('DATA')).
func DATA() (string, error) { return command("DATA") }

// RSET builds the "RSET" request line (Net::SMTP#rset).
func RSET() (string, error) { return command("RSET") }

// QUIT builds the "QUIT" request line (Net::SMTP#quit).
func QUIT() (string, error) { return command("QUIT") }

// STARTTLS builds the "STARTTLS" request line (Net::SMTP#starttls).
func STARTTLS() (string, error) { return command("STARTTLS") }

// command validates a request line and returns the bare reqline, matching the
// argument Ruby's getok/get_response receives (validate_line reqline; the CRLF is
// appended later by writeline, available here via Wire).
func command(line string) (string, error) {
	if err := ValidateLine(line); err != nil {
		return "", err
	}
	return line, nil
}
