// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/xmidt-org/caduceus"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
		}
	}()

	err := caduceus.Caduceus(os.Args[1:], true)

	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(-1)
}
