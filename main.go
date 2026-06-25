// Command terraform-provider-microwave is the entry point for the Microwave
// Terraform provider plugin. Terraform invokes this binary over gRPC; the
// provider implementation lives in internal/provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/microwave-sh/terraform-provider-microwave/internal/provider"
)

// version is overridden at link time by goreleaser via -ldflags "-X main.version=...".
var version = "2.6.0"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Run the provider in debug mode (attach to a running plugin instance for editor/IDE introspection).")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/microwave-sh/microwave",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
