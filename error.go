// Copyright (c) the go-ruby-net-smtp/net-smtp authors
//
// SPDX-License-Identifier: BSD-3-Clause

package netsmtp

import "fmt"

// ErrorKind names a member of the Net::SMTPError family. The host (rbgo) maps
// each kind to the matching Ruby exception class; the codec selects the kind
// purely from the reply code so the mapping needs no interpreter.
type ErrorKind int

const (
	// KindUnknown is Net::SMTPUnknownError (code outside 4xx/5xx, or a 5xx not
	// covered by the more specific kinds).
	KindUnknown ErrorKind = iota
	// KindServerBusy is Net::SMTPServerBusy (4xx).
	KindServerBusy
	// KindSyntax is Net::SMTPSyntaxError (50x).
	KindSyntax
	// KindAuthentication is Net::SMTPAuthenticationError (53x).
	KindAuthentication
	// KindFatal is Net::SMTPFatalError (other 5xx).
	KindFatal
)

// RubyClass is the fully-qualified Ruby exception class name for the kind, the
// constant the host raises. It lets a binding reproduce res.exception_class.
func (k ErrorKind) RubyClass() string {
	switch k {
	case KindServerBusy:
		return "Net::SMTPServerBusy"
	case KindSyntax:
		return "Net::SMTPSyntaxError"
	case KindAuthentication:
		return "Net::SMTPAuthenticationError"
	case KindFatal:
		return "Net::SMTPFatalError"
	default:
		return "Net::SMTPUnknownError"
	}
}

func (k ErrorKind) String() string { return k.RubyClass() }

// SMTPError carries the kind, the offending Response, and the message. It mirrors
// Net::SMTPError instances, whose default message is the reply's first line
// (ProtocolError.new(response).message), so a host can surface the same text.
type SMTPError struct {
	Kind     ErrorKind
	Response *Response
	Msg      string
}

// Error returns the message. When no explicit message was supplied it is the
// response's first line, matching Net::SMTPError's default (the Ruby exception
// message defaults to the response's #message).
func (e *SMTPError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Response != nil {
		return e.Response.Message()
	}
	return e.Kind.RubyClass()
}

// errorFor builds the SMTPError a non-success reply would raise via
// check_response: res.exception_class.new(res).
func errorFor(res *Response) *SMTPError {
	return &SMTPError{Kind: res.ExceptionClass(), Response: res}
}

// continueError builds the SMTPUnknownError check_continue raises when a reply is
// not a 3xx: SMTPUnknownError.new(res, message: "could not get 3xx (#{status}: #{string})").
func continueError(res *Response) *SMTPError {
	return &SMTPError{
		Kind:     KindUnknown,
		Response: res,
		Msg:      fmt.Sprintf("could not get 3xx (%s: %s)", res.Status, res.String),
	}
}
