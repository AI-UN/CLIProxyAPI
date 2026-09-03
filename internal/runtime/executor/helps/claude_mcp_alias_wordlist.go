package helps

import (
	_ "embed"
	"strings"
)

//go:embed claude_alias_brands.txt
var rawClaudeAliasBrands string

//go:embed claude_alias_tlds.txt
var rawClaudeAliasTLDs string

// claudeMCPAliasBrandWords contains recognizable brand and open-source project
// names (2048 entries) used for the first half of the virtual server name and
// the one-word tool ID.
var claudeMCPAliasBrandWords = strings.Fields(rawClaudeAliasBrands)

// claudeMCPAliasTLDWords contains popular top-level-domain words (38 entries)
// used for the second half of the virtual server name.
var claudeMCPAliasTLDWords = strings.Fields(rawClaudeAliasTLDs)
