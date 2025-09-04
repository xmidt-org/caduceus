// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package batch

import ()

func GetBatch[T any](start int, batchSize int, items []T) ([]T, bool) {
	var batch []T
	more := true
	if (len(items) - start) <= batchSize {
		batch = items[start:]
		more = false
	} else {
		batch = items[start : start+batchSize]
	}

	return batch, more
}

func GetBatches[T any](batchSize int, items []T) [][]T {
	batches := [][]T{}

	if len(items) == 0 {
		return batches
	}

	notDone := true
	start := 0
	for notDone {
		items, more := GetBatch(start, batchSize, items)
		notDone = more
		start += batchSize

		batches = append(batches, items)
	}

	return batches

}
