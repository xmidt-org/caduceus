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

func NewRoundTripper(opts ...func(*SRVRecordDialer)) (http.RoundTripper, error) {
	d := NewSRVRecordDialer(opts...)
	if len(d.fqdns) == 0 {
		return http.DefaultTransport, nil
	}

	var errs error

	for _, fqdn := range d.fqdns {
		_, addrs, err := d.resolver.LookupSRV(context.Background(), "", "", fqdn)
		if err != nil {
			errs = errors.Join(errs,
				fmt.Errorf("srv lookup failure: `%s`", fqdn),
				err,
			)
			continue
		}

		d.srvs = append(d.srvs, addrs...)
	}

	// TODO: ask wes/john whether 1 or more net.LookupSRV error should trigger an error from NewSRVRecordDailer
	if len(d.srvs) == 0 {
		return nil, errors.Join(fmt.Errorf("expected atleast 1 srv record from fqdn list `%v`", d.fqdns), errs)
	}

	return &http.Transport{
		DialContext: (d).DialContext,
	}, nil
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
				fmt.Errorf("%v: host `%s` [weight: %d, priortiy: %d] from srv record `%v`",
					err, host, srv.Weight, srv.Priority, d.fqdns))
		} else if conn != nil {
			break
		}
	}

	return conn, errs
}
