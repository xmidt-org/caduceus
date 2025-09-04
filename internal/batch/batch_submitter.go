// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package batch

type BatchSubmitter[T any] interface {
	SubmitBatch(events []T) error
}
