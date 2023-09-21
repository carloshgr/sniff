package main

import (
	"os"
	"log"
	"fmt"
	"regexp"
	
	"github.com/joho/godotenv"
	"github.com/go-resty/resty/v2"
)

func send_request(client *resty.Client, page int, owner string, repo string) *resty.Response {
	resp, err := client.R().
		EnableTrace().
		SetQueryParams(map[string]string{
			"state": "all",
			"sort": "created",
			"per_page": "10",
			"page": fmt.Sprintf("%d", page),
		}).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetAuthToken(os.Getenv("GITHUB_TOKEN")).
		Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/comments", owner, repo))
	
	if err != nil {
		log.Fatal(err)
	}

	return resp
}

func main() {
	godotenv.Load(".env")

	client := resty.New()
	resp := send_request(client, 1, "neovim", "neovim")
	body := string(resp.Body())

	r, _ := regexp.Compile(`\"body\":\"([^\"])*\"`)
	fmt.Println(r.FindAllString(body, -1))
}