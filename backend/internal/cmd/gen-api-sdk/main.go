// Command gen-api-sdk generates mddb's TypeScript client and API reference.
package main

import (
	"fmt"
	"os"

	"github.com/maruel/apisdkgen"
	"github.com/maruel/mddb/backend/internal/server/dto"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-api-sdk:", err)
		os.Exit(1)
	}
}

func generate() error {
	api := apisdkgen.NewAPI("dto", apisdkgen.OutputConfig{
		TypeScriptDir:        "../../../sdk",
		MarkdownDir:          "../../../sdk",
		TypeScriptClientOnly: true,
	}, dto.SDKAPI())
	return apisdkgen.Generate(&api)
}
