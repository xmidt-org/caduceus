package sink

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"

	"github.com/xmidt-org/webhook-schema"
)

func NewSRVRecordDailer(dnsSrvRecord webhook.DNSSrvRecord) (http.RoundTripper, error) {
	if len(dnsSrvRecord.FQDNs) == 0 {
		return http.DefaultTransport, nil
	}

	d := SRVRecordDialer{dnsSrvRecord: dnsSrvRecord}

	var errs error
	for _, fqdn := range d.dnsSrvRecord.FQDNs {
		_, addrs, err := net.LookupSRV("", "", fqdn)
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
		return nil, errors.Join(fmt.Errorf("expected atleast 1 srv record from fqdn list `%v`", d.dnsSrvRecord.FQDNs), errs)
	}

	switch d.dnsSrvRecord.LoadBalancingScheme {
	case "weight":
		sort.Slice(d.srvs, func(i, j int) bool {
			return d.srvs[i].Weight > d.srvs[j].Weight
		})
	case "priortiy":
		sort.Slice(d.srvs, func(i, j int) bool {
			return d.srvs[i].Priority < d.srvs[j].Priority
		})
	default:
		return nil, fmt.Errorf("unknwon loadBalancingScheme type: %s", d.dnsSrvRecord.LoadBalancingScheme)
	}

	return &http.Transport{
		DialContext: (&d).DialContext,
	}, nil

}

type SRVRecordDialer struct {
	srvs         []*net.SRV
	dnsSrvRecord webhook.DNSSrvRecord
}

func (d *SRVRecordDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var errs error
	for _, addr := range d.srvs {
		host := net.JoinHostPort(addr.Target, string(addr.Port))
		conn, err := net.Dial("", host)
		if err != nil {
			errs = errors.Join(errs,
				fmt.Errorf("%v: host `%s` [weight: %s, priortiy: %s] from srv record `%v`",
					err, host, addr.Weight, addr.Priority, d.dnsSrvRecord.FQDNs))
			continue
		}

		return conn, nil
	}

	return nil, errs
}
