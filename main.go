package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {

	webRoot := flag.String("webRoot", "html", "web root directory")
	port := flag.Int("port", 8080, "listen on this port")
	flag.Parse()

	log.Printf("Starting server...\n")
	log.Printf("Set web root: %s\n", *webRoot)

	http.Handle("/websocket", &wsHandler{})

	http.Handle("/", http.FileServer(http.Dir(*webRoot)))

	log.Printf("Listening on :%d...\n", *port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
	if err != nil {
		log.Fatal(err)
	}

}
