// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBatch(t *testing.T) {
	batchSize := 5

	items := []string{}
	batchOfItems, more := GetBatch(0, batchSize, items)
	assert.False(t, more)
	assert.Equal(t, 0, len(batchOfItems))

	items = []string{"a", "b", "c", "d"}
	batchOfItems, more = GetBatch(0, batchSize, items)
	assert.False(t, more)
	assert.Equal(t, 4, len(batchOfItems))

	items = []string{"a", "b", "c", "d", "e"}
	batchOfItems, more = GetBatch(0, batchSize, items)
	assert.False(t, more)
	assert.Equal(t, 5, len(batchOfItems))

	items = []string{"a", "b", "c", "d", "e", "f"}
	batchOfItems, more = GetBatch(0, batchSize, items)
	assert.True(t, more)
	assert.Equal(t, 5, len(batchOfItems))
	batchOfItems, more = GetBatch(0+batchSize, batchSize, items)
	assert.False(t, more)
	assert.Equal(t, 1, len(batchOfItems))
}

func TestGetLargeBatch(t *testing.T) {
	batchSize := 5

	start := 0
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	batchOfItems, more := GetBatch(start, batchSize, items)
	assert.True(t, more)
	assert.Equal(t, 5, len(batchOfItems))

	start += batchSize
	batchOfItems, more = GetBatch(start, batchSize, items)
	assert.True(t, more)
	assert.Equal(t, 5, len(batchOfItems))

	start += batchSize
	batchOfItems, more = GetBatch(start, batchSize, items)
	assert.False(t, more)
	assert.Equal(t, 1, len(batchOfItems))
}

func TestGetBatches100Items(t *testing.T) {
	batchSize := 25

	items := []string{}

	for i := 0; i < 100; i++ {
		items = append(items, fmt.Sprintf("%d", i))
	}

	batches := GetBatches(batchSize, items)
	assert.Equal(t, 4, len(batches))
	assert.Equal(t, 25, len(batches[0]))
	assert.Equal(t, 25, len(batches[1]))
	assert.Equal(t, 25, len(batches[2]))
	assert.Equal(t, 25, len(batches[3]))
}

func TestGetBatchesEmptyItems(t *testing.T) {
	batchSize := 25

	items := []string{}

	batches := GetBatches(batchSize, items)
	assert.Equal(t, 0, len(batches))
}

func TestGetBatchesLessThanOne(t *testing.T) {
	batchSize := 5

	items := []string{"a", "b", "c", "d"}

	batches := GetBatches(batchSize, items)
	assert.Equal(t, 1, len(batches))
	assert.Equal(t, 4, len(batches[0]))
}

func TestGetBatchesExactlyOne(t *testing.T) {
	batchSize := 5

	items := []string{"a", "b", "c", "d", "e"}

	batches := GetBatches(batchSize, items)
	assert.Equal(t, 1, len(batches))
	assert.Equal(t, 5, len(batches[0]))
}

func TestGetBatchesOneOver(t *testing.T) {
	batchSize := 5

	items := []string{"a", "b", "c", "d", "e", "f"}

	batches := GetBatches(batchSize, items)
	assert.Equal(t, 2, len(batches))
	assert.Equal(t, 5, len(batches[0]))
	assert.Equal(t, 1, len(batches[1]))
}
