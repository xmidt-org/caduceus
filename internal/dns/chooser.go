// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"iter"
	"math/rand"
	"net"
	"sort"
)

// Chooser is an iterator that's functionality is to choose one SRV record from a list of SRV records
type Chooser interface {
	Choose() iter.Seq[*net.SRV]
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

	priorityMap := make(map[uint16][]*net.SRV)
	for _, srv := range pc.srvs {
		priorityMap[srv.Priority] = append(priorityMap[srv.Priority], srv)
	}

	seq := func(yield func(*net.SRV) bool) {
		var i uint16 = 1
		var srv *net.SRV
		var srvArr []*net.SRV
		for {
			srvArr = priorityMap[i]
			if len(srvArr) > 1 {
				s, j := getAddrByWeight(srvArr)
				if s == nil {
					i++ //TODO: is this what we want to do or do we want to break?
					continue
				}
				srvArr = append(srvArr[:j], srvArr[j+1:]...)
				priorityMap[i] = srvArr
				srv = s
			} else {
				srv = srvArr[0]
				srvArr = []*net.SRV{}
			}

			if len(srvArr) == 0 {
				i++
			}

			if !yield(srv) {
				break
			}
		}
	}

	return seq
}
func getAddrByWeight(srvs []*net.SRV) (*net.SRV, int) {

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
			return srv, i
		}
	}

	return nil, -1
}

//Do we need this anymore?

// type WeightChooser struct {
// 	srvs []*net.SRV
// }

// func NewWeightChooser(srvs []*net.SRV) *WeightChooser {
// 	return &WeightChooser{
// 		srvs: srvs,
// 	}
// }
// func (wc *WeightChooser) Choose() iter.Seq[*net.SRV] {
// 	//create a copy of d.srvs so that we can edit the srvs list if needed
// 	srvs := make([]*net.SRV, len(wc.srvs))
// 	copy(srvs, wc.srvs)

// 	seq := func(yield func(*net.SRV) bool) {

// 		for {
// 			srv, i := getAddrByWeight(srvs)
// 			if srv == nil {
// 				continue
// 			}

// 			if !yield(srv) {
// 				break
// 			}
// 			srvs = append(srvs[:i], srvs[i+1:]...)
// 		}
// 	}

// 	return seq
// }
