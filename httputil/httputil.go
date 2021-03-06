// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// Package htputil is a toolkit for the Go net/http package.
package httputil

import (
	"net"
	"net/http"
)

// StripPort removes the port specification from an address.
func StripPort(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return s
}

type Error func(w http.ResponseWriter, r *http.Request, status int, err error)
