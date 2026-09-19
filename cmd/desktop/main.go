// Command desktop is the Wails entrypoint for Inferenesia.
// It binds the same core.Service used by the CLI (no parallel agent loop in TS).
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/config"
	"github.com/agamyusliman/inferenesia-app/internal/core"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	serveOnly := flag.Bool("serve", false, "run hub HTTP API only (no WebView); for agent-browser / CI")
	port := flag.Int("port", core.DefaultDevPort, "HTTP port for --serve or api side-channel (4100-4199)")
	configHome := flag.String("config-home", "", "override INFERENESIA_HOME (or legacy YURA_AI_HOME) / config home")
	flag.Parse()

	// Load repo-local .env without printing secrets.
	_ = config.LoadDotEnvFromCWD()

	opts := core.Options{}
	if *configHome != "" {
		opts.ConfigHome = *configHome
	} else if h := os.Getenv(config.EnvHome); h != "" {
		opts.ConfigHome = h
	} else if h := os.Getenv(config.EnvHomeLegacy); h != "" {
		opts.ConfigHome = h
	}

	svc, err := core.NewService(opts)
	if err != nil {
		log.Fatalf("core.NewService: %v", err)
	}
	defer svc.Close()

	// Always start the localhost hub API within mission port range so
	// agent-browser and Vite can call the same core.Service.
	if err := core.ValidateDevPort(*port); err != nil {
		log.Fatal(err)
	}

	// Serve SPA from embedded assets when using --serve (agent-browser path).
	// Wails still uses its own AssetServer when the WebView window is open.
	staticRoot, staticErr := fs.Sub(assets, "frontend/dist")
	if staticErr != nil {
		staticRoot = nil
	}
	url, err := svc.ListenAndServeStatic(*port, staticRoot)
	if err != nil {
		log.Fatalf("hub http: %v", err)
	}
	log.Printf("%s hub API listening on %s", brand.Name, url)

	if *serveOnly {
		// Block forever — process is the HTTP server (+ SPA).
		select {}
	}

	app := NewApp(svc)

	// Title is the product brand (VAL-DESK-001).
	title := brand.Name

	err = wails.Run(&options.App{
		Title:  title,
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 22, B: 28, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		// Prefer a clear message over panic for blank-screen debug.
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		// If assets missing, hint the web build path.
		if _, stErr := assets.Open("frontend/dist/index.html"); stErr != nil {
			wd, _ := os.Getwd()
			fmt.Fprintf(os.Stderr, "hint: build frontend into %s (npm run build in web/)\n",
				filepath.Join(wd, "cmd/desktop/frontend/dist"))
		}
		os.Exit(1)
	}
}
