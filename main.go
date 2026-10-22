package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/spf13/pflag"
)

func main() {
	var listen, configFile string
	pflag.StringVarP(&listen, "listen", "l", ":8080", "HTTP listen address")
	pflag.StringVarP(&configFile, "config", "c", "config.yml", "configuration file")
	pflag.Parse()

	cfg, err := loadConfig(configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := vips.Startup(nil); err != nil {
		log.Fatalf("start libvips: %v", err)
	}
	defer vips.Shutdown()

	server := &http.Server{Addr: listen, Handler: newService(cfg, vipsResizer{})}
	log.Printf("listening on %s with root %s", listen, cfg.Root)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
