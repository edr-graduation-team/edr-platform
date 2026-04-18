package signatures

import "embed"

// builtin_hashes.ndjson is a newline-delimited JSON feed of SHA-256 IOCs.
// The checked-in file is generated from MalwareBazaar public recent export
// (https://bazaar.abuse.ch/export/csv/recent/) — see their FAQ / terms of use.
// Regenerate (from repo root win_edrAgent):
//
//	go run ./tools/gen-malwarebazaar-seed -limit 500 -out internal/signatures/builtin_hashes.ndjson
//
//go:embed builtin_hashes.ndjson
var builtinHashesNDJSON []byte
