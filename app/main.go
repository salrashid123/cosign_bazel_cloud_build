package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"golang.org/x/net/http2"
)

var ()

const ()

func gethandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func main() {

	r := mux.NewRouter()
	r.Methods(http.MethodGet).Path("/").HandlerFunc(gethandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}
	http2.ConfigureServer(server, &http2.Server{})
	fmt.Println("Starting Server..")
	err := server.ListenAndServe()

	fmt.Printf("Unable to start Server %v", err)

}
