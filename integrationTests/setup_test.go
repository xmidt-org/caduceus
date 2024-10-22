// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package integrationtests

import (
	"fmt"
	"os"
	"testing"

	"github.com/xmidt-org/caduceus"
	"github.com/xmidt-org/idock"
)

func TestMain(m *testing.M) {
	if testFlags() == "" {
		fmt.Println("Skipping integration tests.")
		os.Exit(0)
	}

	//os.Setenv("TZ", "UTC/UTC")
	infra := idock.New(
		idock.DockerComposeFile("docker.yml"),
		idock.RequireDockerTCPPorts(4567, 6100, 6101, 6102, 6103, 6600, 6601, 6602, 6603, 4566),
		idock.Program(func() { _ = caduceus.Caduceus([]string{"-f", "caduceus.yml"}, true) }),
		idock.RequireProgramTCPPorts(18111, 18112, 18113),
	)

	err := infra.Start()
	if err != nil {
		panic(err)
	}

	returnCode := m.Run()

	infra.Stop()

	if returnCode != 0 {
		os.Exit(returnCode)
	}
}
