package render

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
)

//go:embed assets/fonts/*.woff2
var fontFS embed.FS

var fontWeights = []string{"400", "500", "600", "700"}

func fontFaceCSS() string {
	var css strings.Builder
	for _, weight := range fontWeights {
		name := "assets/fonts/nunito-latin-" + weight + "-normal.woff2"
		data, err := fontFS.ReadFile(name)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&css, `@font-face {
  font-family: "Nunito";
  font-style: normal;
  font-weight: %s;
  font-display: block;
  src: url(data:font/woff2;base64,%s) format("woff2");
}
`, weight, base64.StdEncoding.EncodeToString(data))
	}
	return css.String()
}
