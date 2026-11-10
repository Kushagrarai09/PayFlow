package main

import (
	"log"
	"net/http"
	"os"

	"payflow/internal/gateway"
)

func main() {
	port := os.Getenv("GATEWAY_PORT")
	if port == "" { port = "8080" }
	routes := map[string]string{
		"/api/auth/": env("AUTH_URL", "http://localhost:8081"),
		"/api/accounts": env("ACCOUNT_URL", "http://localhost:8082"),
		"/api/transactions": env("TRANSACTION_URL", "http://localhost:8084"),
		"/api/ledger/": env("LEDGER_URL", "http://localhost:8083"),
	}
	log.Printf("API gateway listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, gateway.New(routes)))
}
func env(k, fallback string) string { if v:=os.Getenv(k); v!="" { return v }; return fallback }
