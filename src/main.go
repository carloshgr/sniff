package main

import (
	"os"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/go-resty/resty/v2"
)

func main() {
	godotenv.Load(".env")

	client := resty.New()
	resp, err := client.R().
    	EnableTrace().
		SetQueryParams(map[string]string{
			"state": "all",
			"sort": "created",
			"per_page": "1",
			"page": "1",
		}).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetAuthToken(os.Getenv("GITHUB_TOKEN")).
    	Get("https://api.github.com/repos/neovim/neovim/pulls")
	
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(resp)
}