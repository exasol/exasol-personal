// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build darwin && arm64

package localruntimebin

import _ "embed"

//go:embed generated/darwin/arm64/metadata.json
var PayloadMetadata []byte

//go:embed generated/darwin/arm64/payload.tar.gz
var PayloadBundle []byte
