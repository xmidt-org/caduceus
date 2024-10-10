// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package metrics

// Using Caduceus's test suite:
//
// If you are testing a new metric the followng process needs to be done below:
// 1. Create a fake, mockMetric i.e fakeEventType := new(mockCounter)
// 2. If your metric type has yet to be included in mockCaduceusMetricRegistry within mocks.go
//    add your metric type to mockCaduceusMetricRegistry
// 3. Trigger the On method on that "mockMetric" with various different cases of that metric,
//    in both senderWrapper_test.go and/or outboundSender_test.go
//    i.e:
//	    case 1: On("With", []string{"event", iot}
//	    case 2: On("With", []string{"event", unknown}
//   Tests for all possible event_types that will be sent to the metrics Desc.  If all cases arn't
//   included tests will fail.
// 4. Mimic the metric behavior using On i.e if your specific metric is a counter:
//      fakeSlow.On("Add", 1.0).Return()

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	assert := assert.New(t)

	m := Provide()

	assert.NotNil(m)
}

func TestGetDoErrReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "No error",
			err:  nil,
			want: NoErrReason,
		},
		{
			name: "Generic do error",
			err:  errors.New("do_error"),
			want: GenericDoReason,
		},
		{
			name: "Deadline exceeded",
			err:  context.DeadlineExceeded,
			want: deadlineExceededReason,
		},
		{
			name: "Context canceled",
			err:  context.Canceled,
			want: contextCanceledReason,
		},
		// {
		// 	name: "Address error",
		// 	err:  &net.AddrError{},
		// 	want: addressErrReason,
		// },
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDoErrReason(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
