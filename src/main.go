package main

import (
	"io"
	"os"
	"log"
	"fmt"
	"sync"
	"regexp"
	"encoding/csv"

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

func scrape_comments(owner string, repo string, wg *sync.WaitGroup) {
	client := resty.New()
	
	resp := send_request(client, 1, owner, repo)
	body := string(resp.Body())
	r, _ := regexp.Compile(`\"body\":\"([^\"])*\"`)
	
	f, err := os.OpenFile(fmt.Sprintf("data/%s.txt", repo), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	
	for _, v := range r.FindAllString(body, -1) {
		_, err := f.WriteString(fmt.Sprintf("%s\n", v))
		if err != nil {
			log.Fatal(err)
		}
	}

	wg.Done()
}

func main() {
	godotenv.Load(".env")

	f, err := os.Open("repositories.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()	
	
	csvReader := csv.NewReader(f)
	
	_, err = csvReader.Read() // skip first line
	if err != nil {
		if err != io.EOF {
			log.Fatalln(err)
		}
	}

	var wg sync.WaitGroup

	for {
		line, err := csvReader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatal(err)
		}
		
		wg.Add(1)
		go scrape_comments(line[0], line[1], &wg)
	}

	wg.Wait()
}