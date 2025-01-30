// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"errors"
	"iter"
	"math/rand"
	"net"
	"sort"
)
//Chooser is an iterator that's functionality is to choose one SRV record from a list of SRV records
type Chooser interface {
	Choose() iter.Seq[*net.SRV]
}
//enumerated type, with config, unmarshal config to get type, chooser type has method to pick the chooser

type WeightChooser struct {
	srvs []*net.SRV
	errs error
}

func NewWeightChooser(srvs []*net.SRV) *WeightChooser {
	return &WeightChooser{
		srvs: srvs,
	}
}
func (wc *WeightChooser) Choose() iter.Seq[*net.SRV] {
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

type PriorityChooser struct {
	srvs []*net.SRV
}

func NewPriorityChooser(srvs []*net.SRV) *PriorityChooser {
	return &PriorityChooser{
		srvs: srvs,
	}
}

func (pc *PriorityChooser) Choose() iter.Seq[*net.SRV] {
	sort.Slice(pc.srvs, func(i, j int) bool {
		return pc.srvs[i].Priority < pc.srvs[j].Priority
	})

	

	seq := func(yield func(*net.SRV) bool) {
		i := 0
		for {
			srv := pc.srvs[i]
			if !yield(srv) {
				break
			}
			i++
		}
	}

	return seq
}
