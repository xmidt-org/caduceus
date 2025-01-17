// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"errors"
	"iter"
	"net"
	"sort"
)

type Chooser interface {
	choose() iter.Seq[*net.SRV]
}

func NewChooser(srvs []*net.SRV, f func([]*net.SRV) Chooser) Chooser {
	return f(srvs)
}

type WeightChooser struct {
	srvs []*net.SRV
	srv  *net.SRV
	i    int
	errs error
}

func NewWeightChooser(srvs []*net.SRV) *WeightChooser {
	return &WeightChooser{
		srvs: srvs,
	}
}
func (wc *WeightChooser) choose() iter.Seq[*net.SRV] {
	//create a copy of d.srvs so that we can edit the srvs list if needed
	srvs := make([]*net.SRV, len(wc.srvs))
	copy(srvs, wc.srvs)

	seq := func(yield func(*net.SRV) bool) {

		for {
			srv, i, err := getAddrByWeight(srvs)
			if err != nil {
				wc.errs = errors.Join(wc.errs, err)
				break
			}
			if !yield(srv) {
				break
			}
			srvs = append(srvs[:i], srvs[i+1:]...)
		}
	}

	return seq
}

type PriorityChooser struct {
	srvs   []*net.SRV
	srv    *net.SRV
	sorted bool
	i      int
}

func NewPriorityChooser(srvs []*net.SRV) *PriorityChooser {
	return &PriorityChooser{
		srvs: srvs,
	}
}

func (pc *PriorityChooser) choose() iter.Seq[*net.SRV] {
	if !pc.sorted {
		sort.Slice(pc.srvs, func(i, j int) bool {
			return pc.srvs[i].Priority < pc.srvs[j].Priority
		})
		pc.sorted = true
	}
	pc.srv = pc.srvs[pc.i]
	pc.i++
}
