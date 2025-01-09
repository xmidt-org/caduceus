package dns

import (
	"context"
	"errors"
	"fmt"
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

	d := SRVRecordDialer{fqdns: fqdns}

	var errs error
	for _, fqdn := range d.fqdns {
		_, addrs, err := resolver.LookupSRV(context.Background(),"", "", fqdn)
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

	switch sortBy {
	case "weight":
		sort.Slice(d.srvs, func(i, j int) bool {
			return d.srvs[i].Weight > d.srvs[j].Weight
		})
	case "priority":
		sort.Slice(d.srvs, func(i, j int) bool {
			return d.srvs[i].Priority < d.srvs[j].Priority
		})
	default:
		return nil, fmt.Errorf("unknown loadBalancingScheme type: %s", sortBy)
	}

	return &http.Transport{
		DialContext: (&d).DialContext,
	}, nil

}

type SRVRecordDialer struct {
	srvs  []*net.SRV
	fqdns []string
}

func (d *SRVRecordDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var errs error
	for _, addr := range d.srvs {
		host := net.JoinHostPort(addr.Target, fmt.Sprint(addr.Port))
		conn, err := net.Dial("", host)
		if err != nil {
			errs = errors.Join(errs,
				fmt.Errorf("%v: host `%s` [weight: %d, priortiy: %d] from srv record `%v`",
					err, host, addr.Weight, addr.Priority, d.fqdns))
			continue
		}

		return conn, nil
	}

	return nil, errs
}
