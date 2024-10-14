// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"fmt"
	"hash/crc32"
	"reflect"
	"sort"

	"github.com/xmidt-org/wrp-go/v3"
)

type Node struct {
	hash int
	sink string
}

type HashRing []Node

func (h HashRing) Len() int {
	return len(h)
}
func (h HashRing) Less(i, j int) bool {
	return h[i].hash < h[j].hash
}
func (h HashRing) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h HashRing) Get(key string) string {
	if len(h) == 0 {
		return ""
	}
	hash := int(crc32.ChecksumIEEE([]byte(key)))
	idx := sort.Search(len(h), func(i int) bool {
		return h[i].hash >= hash
	})
	if idx == len(h) {
		idx = 0
	}
	return h[idx].sink
}

func (h *HashRing) Add(server string) {
	hash := int(crc32.ChecksumIEEE([]byte(server)))
	node := Node{hash: hash, sink: server}
	*h = append(*h, node)
	sort.Sort(h)
}

func (h *HashRing) Remove(server string) {
	hash := int(crc32.ChecksumIEEE([]byte(server)))
	for i, node := range *h {
		if node.hash == hash {
			*h = append((*h)[:i], (*h)[i+1:]...)
			break
		}
	}
	sort.Sort(h)
}

func GetKey(field string, msg *wrp.Message) string {

	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem() // Dereference pointer if necessary
	}

	value := v.FieldByName(field)
	if value.IsValid() {
		return fmt.Sprintf("%v", value.Interface())
	}

	return ""

}
