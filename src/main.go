package main

import (
	"io"
	"os"
	"log"
	"fmt"
	"sync"
	"regexp"
	"strings"
	"encoding/csv"

	"github.com/joho/godotenv"
	"github.com/go-resty/resty/v2"
)

func sendRequest(client *resty.Client, queryParams map[string]string, url string) *resty.Response {
	resp, err := client.R().
		EnableTrace().
		SetQueryParams(queryParams).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetAuthToken(os.Getenv("GITHUB_TOKEN")).
		Get(url)
	
	if err != nil {
		log.Fatal(err)
	}

	return resp
}

func sendRequests(owner string, repo string, respCh chan *resty.Response) {
	client := resty.New()

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/comments", owner, repo)

	var resp *resty.Response
	page := 1
	for {
		queryParams := map[string]string{
			"state": "all",
			"sort": "created",
			"per_page": "100",
			"page": fmt.Sprintf("%d", page),
		}
		
		for {
			resp = sendRequest(client, queryParams, url)

			if resp.IsSuccess() {
				log.Println(fmt.Sprintf("Request, %s, %d, success", url, page))
				break
			} else {
				log.Println(fmt.Sprintf("Request, %s, %d, failed, %s", url, page, resp.Body()))
			}
		}
		
		if resp.Size() <= 2 {
			log.Println(fmt.Sprintf("Response, %s, %d, %s", url, page, resp.Body()))
			break
		}

		respCh <- resp
		page = page+1
	}

	close(respCh)
}

func processResponses(respCh chan *resty.Response, commCh chan string) {
	content_regex := regexp.MustCompile(`"body":"(?:[^"\\]|\\.)*"`)
	
	for response := range respCh {
		body := string(response.Body())

		for _, comment := range content_regex.FindAllString(body, -1) {
			commCh <- strings.TrimPrefix(fmt.Sprintf("%s\n", comment), "\"body\":")
		}
	}

	close(commCh)
}

func writeComments(owner string, repo string, commCh chan string, done chan bool) {
	f, err := os.OpenFile(fmt.Sprintf("data/%s-%s.csv", owner, repo), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	
	for comment := range commCh {
		_, err := f.WriteString(comment)
		if err != nil {
			log.Fatal(err)
		}
	}

	done <- true
}

func scrapeComments(owner string, repo string, wg *sync.WaitGroup) {	
	respCh := make(chan *resty.Response)
	commCh := make(chan string)
	done := make(chan bool)
	go sendRequests(owner, repo, respCh)
	go processResponses(respCh, commCh)
	go writeComments(owner, repo, commCh, done)

	<-done

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

	os.RemoveAll("./data")
	os.Mkdir("./data", os.ModePerm)

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
		go scrapeComments(line[0], line[1], &wg)
	}

	wg.Wait()
}