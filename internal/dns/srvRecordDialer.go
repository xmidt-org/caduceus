// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sort"
)

type Resolver interface {
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

func NewSRVRecordDialer(fqdns []string, sortBy string, resolver Resolver) (http.RoundTripper, error) {
	if len(fqdns) == 0 {
		return http.DefaultTransport, nil
	}

	if resolver == nil {
		resolver = net.DefaultResolver
	}

	d := SRVRecordDialer{
		fqdns:  fqdns,
		sortBy: sortBy,
	}

	var errs error

	for _, fqdn := range d.fqdns {
		_, addrs, err := resolver.LookupSRV(context.Background(), "", "", fqdn)
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
		DialContext: (&d).DialContext,
	}, nil

}

type SRVRecordDialer struct {
	srvs   []*net.SRV
	fqdns  []string
	sortBy string
}

func (d *SRVRecordDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var errs error
	var err error
	var conn net.Conn

	//TODO: add retry logic if we receive conn error? or just move to next one?
		for conn == nil {
			addr, i, err := getAddrByWeight(srvs)
			if err != nil {
				errs = errors.Join(errs, err)
				break
			}
			host := net.JoinHostPort(addr.Target, fmt.Sprint(addr.Port))
			conn, err = net.Dial("tcp", host) //TODO: make network variable configurable
			if err != nil {
				errs = errors.Join(errs,
					fmt.Errorf("%v: host `%s` [weight: %d, priortiy: %d] from srv record `%v`",
						err, host, addr.Weight, addr.Priority, d.fqdns))
				srvs = append(srvs[:i], srvs[i+1:]...)
			}
		}
	case "priority":
		sort.Slice(d.srvs, func(i, j int) bool {
			return d.srvs[i].Priority < d.srvs[j].Priority
		})

		for _, addr := range d.srvs {
			host := net.JoinHostPort(addr.Target, fmt.Sprint(addr.Port))
			conn, err = net.Dial("tcp", host) //TODO: make network variable configurable
			if err != nil {
				errs = errors.Join(errs,
					fmt.Errorf("%v: host `%s` [weight: %d, priortiy: %d] from srv record `%v`",
						err, host, addr.Weight, addr.Priority, d.fqdns))
				continue
			}
			return conn, errs
		}
	default:
		return nil, fmt.Errorf("unknown loadBalancingScheme type: %s", d.sortBy)
	}

	return conn, errs
}
func getAddrByWeight(srvs []*net.SRV) (*net.SRV, int, error) {
	if len(srvs) == 0 {
		return nil, -1, errors.New("no SRV records available")
	}

	totalWeight := 0
	for _, srv := range srvs {
		totalWeight += int(srv.Weight)
	}

	if totalWeight == 0 {
		totalWeight = len(srvs)
	}

	randWeight := rand.Intn(totalWeight)
	currentWeight := 0

	for i, srv := range srvs {
		currentWeight += int(srv.Weight)
		if randWeight < currentWeight {
			return srv, i, nil
		}
	}

	return nil, -1, errors.New("failed to choose an SRV record by weight")
}
