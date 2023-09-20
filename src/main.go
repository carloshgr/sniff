package main

import (
	"fmt"
	"github.com/go-resty/resty/v2"
)

func main() {
	client := resty.New()

	resp, err := client.R().
    	EnableTrace().
    	Get("https://api.github.com/octocat")
	
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(resp)
}