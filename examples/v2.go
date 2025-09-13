package main

import (
	"fmt"

	v2 "github.com/oarkflow/ip/v2"
)

func main() {
	ip := "103.190.40.32"
	response := v2.Lookup(ip)
	if response.Found {
		fmt.Printf("%s -> %s, %s, %s, %s (%.4f, %.4f)\n",
			ip, response.CountryCode, response.Country, response.Region, response.City, response.Latitude, response.Longitude)
	} else {
		fmt.Printf("%s -> Not found\n", ip)
	}
}
