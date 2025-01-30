// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

var (
	errNoSrvRecordFmt = "expected atleast 1 srv record from fqdn list `%v`"
	errSrvLookUpFmt   = "srv lookup failure `%s`:`%v`"
	errDialConnFmt    = "%v: host `%s` [weight: %d, priortiy: %d] from srv record `%v`"
)

type Resolver interface {
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

func NewSRVRecordDialer(opts ...func(*SRVRecordDialer)) *SRVRecordDialer {
	dialer := &SRVRecordDialer{}

	for _, o := range opts {
		o(dialer)
	}

	return dialer
}

func WithFQDNS(fqdns []string) func(*SRVRecordDialer) {
	return func(d *SRVRecordDialer) {
		d.fqdns = fqdns
	}
}

func WithResolver(resolver Resolver) func(*SRVRecordDialer) {
	return func(d *SRVRecordDialer) {
		if resolver == nil {
			d.resolver = net.DefaultResolver
		} else {
			d.resolver = resolver
		}
	}
}

func WithChooser(chooser Chooser) func(*SRVRecordDialer) {
	return func(d *SRVRecordDialer) {
		d.chooser = chooser
	}
}

func NewCustomTransport(opts ...func(*http.Transport) error) (http.RoundTripper, error) {
	t := &http.Transport{}
	var err error

	for _, o := range opts {
		e := o(t)
		if e != nil {
			err = errors.Join(err, e)
		}
	}

	return t, err
}

func WithCustomDialer(dialer *SRVRecordDialer) func(*http.Transport) error {
	var errs error

	for _, fqdn := range dialer.fqdns {
		_, addrs, err := dialer.resolver.LookupSRV(context.Background(), "", "", fqdn)
		if err != nil {
			errs = errors.Join(errs,
				fmt.Errorf(errSrvLookUpFmt, fqdn, err),
			)
			continue
		}

		dialer.srvs = append(dialer.srvs, addrs...)
	}
	if len(dialer.srvs) == 0 {
		return func(t *http.Transport) error {
			return errors.Join(fmt.Errorf(errNoSrvRecordFmt, dialer.fqdns), errs)
		}
	}

	return func(t *http.Transport) error {
		t.DialContext = (dialer).DialContext
		return errs
	}
}

type SRVRecordDialer struct {
	srvs     []*net.SRV
	fqdns    []string
	resolver Resolver
	chooser  Chooser
}

func (d *SRVRecordDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var errs error
	var err error
	var conn net.Conn

	//TODO: add retry logic if we receive conn error? or just move to next one?
	for srv := range d.chooser.Choose() {
		host := net.JoinHostPort(srv.Target, fmt.Sprint(srv.Port))
		conn, err = net.Dial("tcp", host) //TODO: make network variable configurable
		if err != nil {
			errs = errors.Join(errs,
				fmt.Errorf(errDialConnFmt,
					err, host, srv.Weight, srv.Priority, d.fqdns))
		} else if conn != nil {
			break
		}
	}

	return conn, errs
}
