package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCaduceusRing(t *testing.T) {
	assert := assert.New(t)

	testCaduceusRing := NewCaduceusRing(10)

	assert.NotNil(testCaduceusRing)
}

func TestCaduceusRing(t *testing.T) {
	assert := assert.New(t)

	testCaduceusRing := NewCaduceusRing(10)
	testCaduceusRing.Add("test1")
	testCaduceusRing.Add("test2")

	t.Run("TestCaduceusRingSnapshot", func(t *testing.T) {
		testValues := testCaduceusRing.Snapshot()
		assert.Equal(2, len(testValues))
		assert.Equal("test1", testValues[0].(string))
		assert.Equal("test2", testValues[1].(string))
	})
}
