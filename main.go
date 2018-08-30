// Mumblet a web-based Mumble client

/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

	log.Printf("Listening on port :%d\n", *port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
	if err != nil {
		log.Fatal(err)
	}
}
