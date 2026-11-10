package main

import (
	"log"
	"net/http"
	"os"
	"payflow/internal/services"
)

func main() {
	name := os.Getenv("SERVICE")
	port := os.Getenv("PORT")
	if port == "" { port = "8081" }
	handler, err := services.New(name)
	if err != nil { log.Fatal(err) }
	log.Printf("%s service listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
