// Command gen-api-sdk generates mddb's TypeScript DTOs, client, and API reference.
package main

import (
	"flag"
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
	check := flag.Bool("check", false, "verify generated SDKs without writing them")
	flag.Parse()
	api := apisdkgen.NewAPI("dto", apisdkgen.OutputConfig{
		TypeScriptDir: "../../../sdk",
		MarkdownDir:   "../../../sdk",
	}, dto.SDKAPI())
	if *check {
		return api.Verify()
	}
	return api.Generate()
}
