package main

import (
	"embed"
	"flag"
	"fmt"
	"net/http"

	"github.com/asiffer/svgg/api"
)

const BANNER = "\033[33m" + `
███████╗██╗   ██╗ ██████╗  ██████╗ 
██╔════╝██║   ██║██╔════╝ ██╔════╝ 
███████╗██║   ██║██║  ███╗██║  ███╗
╚════██║╚██╗ ██╔╝██║   ██║██║   ██║
███████║ ╚████╔╝ ╚██████╔╝╚██████╔╝
╚══════╝  ╚═══╝   ╚═════╝  ╚═════╝ 
` + "\033[0m"

//go:embed static
var static embed.FS

// config
var (
	host string = "127.0.0.1"
	port uint   = 4444
)

func main() {
	flag.StringVar(&host, "host", "127.0.0.1", "Listening IP")
	flag.UintVar(&port, "port", 4444, "Listing port")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", host, port)

	fmt.Println(BANNER)
	api.Info.Printf("Server starts. Service is available at http://%s\n", addr)

	mux := http.NewServeMux()
	mux.Handle(api.STATIC_URL, http.FileServer(http.FS(static)))
	mux.HandleFunc("/", api.Handler)

	if err := http.ListenAndServe(addr, mux); err != nil {
		api.Error.Panic(err)
	}
}
