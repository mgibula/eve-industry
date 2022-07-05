package config

import "flag"

var (
	CookDb    = flag.String("cook-database", "", "EVE DB to cook from")
	ClientId  = flag.String("client-id", "", "EVE API Client ID")
	SecretKey = flag.String("secret-key", "", "EVE API Secret Key")
)

func init() {
	flag.Parse()
}
