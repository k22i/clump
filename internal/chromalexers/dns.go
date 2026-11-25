package chromalexers

import (
	chroma "github.com/alecthomas/chroma/v2"
	"regexp"
)

// TODO(moorereason): can this be factored away?
var zoneAnalyserRe = regexp.MustCompile(`(?m)^@\s+IN\s+SOA\s+`)

func analyserDNS(reg *chroma.LexerRegistry) {
	reg.Get("dns").SetAnalyser(func(text string) float32 {
		if zoneAnalyserRe.FindString(text) != "" {
			return 1.0
		}
		return 0.0
	})
}
