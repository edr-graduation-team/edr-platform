//go:build windows

package responder

import _ "embed"

//go:embed default_process_rules.json
var defaultProcessRulesJSON []byte
