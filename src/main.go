package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
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

func getRate(rateCh chan int) {
	sleepTime := time.Second

	for {
		rateCh <- 1
		time.Sleep(sleepTime)
	}
}

func getRemainingLimit(limitCh chan int) {
	client := resty.New()

	var body map[string]interface{}
	for {
		url := "https://api.github.com/rate_limit"

		resp := sendRequest(client, make(map[string]string), url)

		json.Unmarshal(resp.Body(), &body)

		remaining := int(math.Round(body["resources"].(map[string]interface{})["core"].(map[string]interface{})["remaining"].(float64)))

		for i := 0; i < remaining; i++ {
			limitCh <- 1
		}
	}
}

func sendRequests(owner string, repo string, respCh chan *resty.Response, limitCh chan int, rateCh chan int) {
	client := resty.New()

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/comments", owner, repo)

	var resp *resty.Response
	page := 1
	for {
		queryParams := map[string]string{
			"state":    "all",
			"sort":     "created",
			"per_page": "100",
			"page":     fmt.Sprintf("%d", page),
		}

		for {
			<-limitCh
			<-rateCh
			resp = sendRequest(client, queryParams, url)

			if resp.IsSuccess() {
				log.Printf("Request, %s, %d, success", url, page)
				break
			} else {
				log.Printf("Request, %s, %d, failed, %s", url, page, resp.Body())
			}
		}

		if resp.Size() <= 2 {
			log.Printf("Response, %s, %d, %s", url, page, resp.Body())
			break
		}

		respCh <- resp
		page = page + 1
	}

	close(respCh)
}

func processResponses(respCh chan *resty.Response, commCh chan []string) {
	var comments interface{}
	var user_id,
		user_login,
		pull_request_url,
		comment_id,
		created_at,
		path, diff_hunk,
		content string

	replacer := strings.NewReplacer("\r", "", "\n", "")

	for response := range respCh {
		json.Unmarshal(response.Body(), &comments)
		comments, _ := comments.([]interface{})

		for _, comment := range comments {
			comment_map, _ := comment.(map[string]interface{})

			user := comment_map["user"]
			if user != nil {
				user_id = fmt.Sprintf("%d", int(user.(map[string]interface{})["id"].(float64)))
				user_login = user.(map[string]interface{})["login"].(string)
			}

			pull_request_url = comment_map["pull_request_url"].(string)
			comment_id = fmt.Sprintf("%d", int(comment_map["id"].(float64)))
			created_at = comment_map["created_at"].(string)
			path = comment_map["path"].(string)
			diff_hunk = replacer.Replace(comment_map["diff_hunk"].(string))
			content = replacer.Replace(comment_map["body"].(string))

			data := []string{
				user_id,
				user_login,
				pull_request_url,
				comment_id,
				created_at,
				path,
				diff_hunk,
				content}
			commCh <- data
		}
	}

	close(commCh)
}

func writeComments(owner string, repo string, commCh chan []string, done chan bool) {
	f, err := os.OpenFile(fmt.Sprintf("data/%s-%s.csv", owner, repo), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)

	writer.Write([]string{
		"user_id",
		"user_login",
		"pull_request_url",
		"comment_id",
		"created_at",
		"path",
		"diff_hunk",
		"content"})

	for comment := range commCh {
		err := writer.Write(comment)
		if err != nil {
			log.Fatal(err)
		}
	}

	done <- true
}

func scrapeComments(owner string, repo string, limitCh chan int, rateCh chan int, wg *sync.WaitGroup) {
	respCh := make(chan *resty.Response)
	commCh := make(chan []string)
	done := make(chan bool)
	go sendRequests(owner, repo, respCh, limitCh, rateCh)
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

	limitCh := make(chan int, 5000)
	rateCh := make(chan int, 5)
	go getRemainingLimit(limitCh)
	go getRate(rateCh)

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
		go scrapeComments(line[0], line[1], limitCh, rateCh, &wg)
	}

	wg.Wait()
}
