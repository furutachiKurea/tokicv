package render

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

//go:embed assets/pdfjs/pdf.min.mjs assets/pdfjs/pdf.worker.min.mjs
var pdfjsFS embed.FS

const pdfjsPNGScale = 2.0

func pdfjsAsset(name string) string {
	data, err := pdfjsFS.ReadFile("assets/pdfjs/" + name)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// PDFJSPrepareScript loads the bundled pdfjs-dist module and worker from Blob
// URLs, reads the printed PDF bytes, and returns its exact Sheet count.
func PDFJSPrepareScript(pdf []byte) string {
	module := jsString(pdfjsAsset("pdf.min.mjs"))
	worker := jsString(pdfjsAsset("pdf.worker.min.mjs"))
	data := jsString(base64.StdEncoding.EncodeToString(pdf))
	return fmt.Sprintf(`(async () => {
  const moduleURL = URL.createObjectURL(new Blob([%s], {type: "text/javascript"}));
  const workerURL = URL.createObjectURL(new Blob([%s], {type: "text/javascript"}));
  try {
    const pdfjs = await import(moduleURL);
    pdfjs.GlobalWorkerOptions.workerSrc = workerURL;
    const bytes = Uint8Array.from(atob(%s), c => c.charCodeAt(0));
    const pdfDocument = await pdfjs.getDocument({
      data: bytes,
      isEvalSupported: false,
      useSystemFonts: true
    }).promise;
    globalThis.__tokicvPDFJS = {document: pdfDocument, moduleURL, workerURL};
    return pdfDocument.numPages;
  } catch (error) {
    URL.revokeObjectURL(moduleURL);
    URL.revokeObjectURL(workerURL);
    throw error;
  }
})()`, module, worker, data)
}

// PDFJSRenderPageScript rasterizes one printed PDF Sheet as a base64 PNG.
func PDFJSRenderPageScript(index int) string {
	return fmt.Sprintf(`(async () => {
  const pdfDocument = globalThis.__tokicvPDFJS.document;
  const page = await pdfDocument.getPage(%d);
  const viewport = page.getViewport({scale: %s});
  const canvas = document.createElement("canvas");
  canvas.width = Math.ceil(viewport.width);
  canvas.height = Math.ceil(viewport.height);
  const context = canvas.getContext("2d");
  context.fillStyle = "#ffffff";
  context.fillRect(0, 0, canvas.width, canvas.height);
  await page.render({
    canvasContext: context,
    viewport: viewport,
    intent: "print"
  }).promise;
  page.cleanup();
  const data = canvas.toDataURL("image/png");
  return data.substring(data.indexOf(",") + 1);
})()`, index+1, formatNumber(pdfjsPNGScale))
}

func jsString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
