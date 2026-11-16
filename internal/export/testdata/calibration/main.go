package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"tokicv/internal/export"
	"tokicv/internal/render"
)

func main() {
	input := flag.String("input", "", "Resume Document input path")
	output := flag.String("output", "", "PDF output path")
	font := flag.String("font", string(render.FontNoto), "font stack")
	lineHeight := flag.Float64("line-height", 1.40, "unitless line height")
	marginTop := flag.Float64("margin-top", 0, "top margin in millimetres")
	marginBottom := flag.Float64("margin-bottom", 0, "bottom margin in millimetres")
	marginV := flag.Float64("margin-v", 10, "top and bottom margins in millimetres")
	sectionGap := flag.Float64("section-gap", 12, "section gap in CSS pixels")
	itemGap := flag.Float64("item-gap", 5, "item gap in CSS pixels")
	flag.Parse()

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "input and output are required")
		os.Exit(2)
	}
	fontStack, ok := render.ParseFontStack(*font)
	if !ok {
		fmt.Fprintf(os.Stderr, "invalid font stack %q\n", *font)
		os.Exit(2)
	}
	layout := render.DefaultLayout()
	layout.FontStack = fontStack
	layout.LineHeight = *lineHeight
	layout.MarginVMM = *marginV
	layout.MarginTopMM = *marginTop
	layout.MarginBottomMM = *marginBottom
	layout.SectionGapPX = *sectionGap
	layout.ItemGapPX = *itemGap

	if err := export.Run(export.Options{
		Input:     *input,
		Output:    *output,
		Timeout:   30 * time.Second,
		Layout:    layout,
		LayoutSet: true,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
