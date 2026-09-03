//go:build js && wasm

package attractor

import (
	_ "embed"
	"encoding/base64"
	"strings"
)

// Embedded UI fonts (all SIL Open Font License), injected as base64 @font-face
// rules so the served page stays a single self-contained file:
//
//	DSEG7 Classic — 7-segment digits for the angle LED readouts
//	B612 Mono     — the panel body font (designed for cockpit legibility)
//	Chakra Petch  — section headers (technical/lab look)
var (
	//go:embed fonts/dseg7.woff2
	fontDSEG7 []byte
	//go:embed fonts/b612-400.woff2
	fontB612Regular []byte
	//go:embed fonts/b612-700.woff2
	fontB612Bold []byte
	//go:embed fonts/chakra-600.woff2
	fontChakra []byte
)

func fontFace(family, weight string, data []byte) string {
	return "@font-face{font-family:'" + family + "';font-style:normal;font-weight:" + weight +
		";font-display:swap;src:url(data:font/woff2;base64," +
		base64.StdEncoding.EncodeToString(data) + ") format('woff2');}"
}

// injectFonts adds the @font-face rules to the document head. Called once from
// Run before the panel is styled, so the CSS font-family names resolve.
func injectFonts() {
	var b strings.Builder
	b.WriteString(fontFace("DSEG7 Classic", "700", fontDSEG7))
	b.WriteString(fontFace("B612 Mono", "400", fontB612Regular))
	b.WriteString(fontFace("B612 Mono", "700", fontB612Bold))
	b.WriteString(fontFace("Chakra Petch", "600", fontChakra))
	st := doc.Call("createElement", "style")
	st.Set("textContent", b.String())
	doc.Get("head").Call("appendChild", st)
}
