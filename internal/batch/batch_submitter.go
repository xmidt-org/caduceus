// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package batch

type BatchSubmitter[T any] interface {
	SubmitBatch(events []T) error
}
