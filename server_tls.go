// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/x509"

	"github.com/xmidt-org/webpa-common/v2/server"
)

// MtlsConfig controls client certificate requirements on the primary server TLS listener.
//
// DisableRequire and DisableVerify combine to select tls.ClientAuthType:
//   - DisableRequire=false, DisableVerify=false => tls.RequireAndVerifyClientCert
//   - DisableRequire=true,  DisableVerify=false => tls.VerifyClientCertIfGiven
//   - DisableRequire=false, DisableVerify=true  => tls.RequireAnyClientCert
//   - DisableRequire=true,  DisableVerify=true  => tls.RequestClientCert
//
// Note: webpa-common only exposes tls.RequireAndVerifyClientCert when a CA cert file is
// provided. DisableRequire has no effect at the listener level; DisableVerify bypasses
// the VerifyPeerCertificate callback only.
type MtlsConfig struct {
	ClientCACertificateFile string `mapstructure:"clientCACertificateFile"`
	DisableRequire          bool   `mapstructure:"disableRequire"`
	DisableVerify           bool   `mapstructure:"disableVerify"`
}

// applyPrimaryMtls applies mTLS settings to the primary server before Prepare is called.
func applyPrimaryMtls(webPA *server.WebPA, mtls *MtlsConfig) {
	if mtls == nil {
		return
	}
	webPA.Primary.ClientCACertFile = mtls.ClientCACertificateFile
	if mtls.DisableVerify {
		// skip the peer certificate verification callback
		webPA.Primary.SetPeerVerifyCallback(func(_ [][]byte, _ [][]*x509.Certificate) error {
			return nil
		})
	}
}
