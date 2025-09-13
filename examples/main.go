package main

import (
	"fmt"

	"github.com/oarkflow/ip"
)

func main() {
	ip.Init()
	fmt.Println(ip.Country("27.34.68.218"))

	fmt.Println(ip.Lookup(ip.FromHeader("27.34.68.218", func(name string) string {
		return "108.28.159.21"
	})))

	ipStr := "103.190.40.32"
	response := ip.Lookup(ipStr)
	if response.Found {
		fmt.Printf("%s -> %s, %s, %s, %s (%.4f, %.4f)\n",
			ipStr, response.CountryCode, response.Country, response.Region, response.City, response.Latitude, response.Longitude)
	} else {
		fmt.Printf("%s -> Not found\n", ipStr)
	}
}
