package main

import (
	"log"

	"github.com/mgibula/eve-industry/server"
	"github.com/mgibula/eve-industry/server/config"
	"github.com/mgibula/eve-industry/server/db"
)

func main() {
	if *config.CookDb != "" {
		log.Println("Cooking database")

		db.CookDatabase(*config.CookDb)
	} else {
		log.Println("Starting EVE Industry Manager")

		s := server.CreateServer()
		s.Run(":8080")
	}
}
