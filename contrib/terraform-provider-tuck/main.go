// Command terraform-provider-tuck is the Terraform provider for the Tuck
// secrets manager. It exposes tuck_kv_secret (resource + data source) and
// tuck_policy (resource) to Terraform configurations.
//
// Registry address: registry.terraform.io/NAGenaev/tuck
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"terraform-provider-tuck/internal/provider"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "start provider in debug mode for use with a debugger")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/NAGenaev/tuck",
		Debug:   debug,
	}
	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err)
	}
}
