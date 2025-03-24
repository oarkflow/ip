package main

import (
	"fmt"

	"github.com/oarkflow/ip"
)

func main() {
	fmt.Println(ip.Country("27.34.68.218"))

	fmt.Println(ip.GeoLocation(ip.FromHeader("27.34.68.218", func(name string) string {
		return "108.28.159.21"
	})))
}
