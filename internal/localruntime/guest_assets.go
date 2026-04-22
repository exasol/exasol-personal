// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import _ "embed"

//go:embed guest/profile.sh
var guestBootstrapProfile []byte

//go:embed guest/exasol-localruntime-entrypoint.sh
var guestEntrypointWrapper []byte
