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
	url := "https://api.github.com/rate_limit"

	var body map[string]interface{}
	for {
		resp := sendRequest(client, make(map[string]string), url)

		json.Unmarshal(resp.Body(), &body)

		remaining := int(math.Round(body["resources"].(map[string]interface{})["core"].(map[string]interface{})["remaining"].(float64)))
		reset := int(math.Round(body["resources"].(map[string]interface{})["core"].(map[string]interface{})["reset"].(float64)))

		for i := 0; i < remaining; i++ {
			limitCh <- 1
		}

		now := time.Now().Unix()
		sleepTime := int64(reset) - now

		log.Printf("%d requests remaining, reset in %d seconds", remaining, sleepTime)

		time.Sleep(time.Duration(sleepTime) * time.Second)

	}
}

func sendRequests(url string, respCh chan *resty.Response, limitCh chan int, rateCh chan int) {
	client := resty.New()

	var resp *resty.Response
	page := 1
	
	queryParams := map[string]string{}

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

	respCh <- resp

	close(respCh)
}

func processCommentResponse(respCh chan *resty.Response, commentCh chan map[string]interface{}) {
	var comment interface{}
	for response := range respCh {
		json.Unmarshal(response.Body(), &comment)
		comment, _ := comment.(map[string]interface{})
		commentCh <- comment
	}
}

func filterFiles(filesCh chan []interface{}, commentCh chan map[string]interface{}, dataCh chan []string) {
	comment := <- commentCh
	files := <- filesCh

	replacer := strings.NewReplacer("\r", "", "\n", "")
	var data []string

	for _, file := range files {
		fileMap, _ := file.(map[string]interface{})

		if fileMap["filename"] == comment["path"] {
			data = append(data, fmt.Sprintf("%d", int(comment["id"].(float64))))
			data = append(data, replacer.Replace(comment["diff_hunk"].(string)))
			data = append(data, fileMap["filename"].(string))
			data = append(data, fileMap["blob_url"].(string))
			data = append(data, fileMap["raw_url"].(string))
		}
	}
	dataCh <- data
}

func processFilesResponse(respCh chan *resty.Response, filesCh chan []interface{}) {
	var files interface{}
	for response := range respCh {
		json.Unmarshal(response.Body(), &files)
		files, _ := files.([]interface{})
		filesCh <- files
	}
}

func writeComments(writer *csv.Writer, dataCh chan []string, done chan bool) {
	data := <- dataCh

	if len(data) > 0 {
		err := writer.Write(data)
		if err != nil {
			log.Fatal(err)
		}
	}
	
	done <- true
}

func getFileUrl(prUrl string, commentUrl string, limitCh chan int, rateCh chan int, control chan bool, writer *csv.Writer, wg *sync.WaitGroup) {
	commentRespCh := make(chan *resty.Response)
	filesRespCh := make(chan *resty.Response)
	commentCh := make(chan map[string]interface{})
	filesCh := make(chan []interface{})
	done := make(chan bool)
	dataCh := make(chan []string)

	filesUrl := fmt.Sprintf("%s/files", prUrl)
	go sendRequests(commentUrl, commentRespCh, limitCh, rateCh)
	go sendRequests(filesUrl, filesRespCh, limitCh, rateCh)
	go processCommentResponse(commentRespCh, commentCh)
	go processFilesResponse(filesRespCh, filesCh)
	go filterFiles(filesCh, commentCh, dataCh)
	go writeComments(writer, dataCh, done)

	<-done
	control <- true
	wg.Done()
}

func createCommentUrl(prUrl string, commentId string) string {
	data := strings.Split(prUrl, "/")
	return fmt.Sprintf("%s/%s/comments/%s", data[0], strings.Join(data[1:len(data)-1], "/"), commentId)
}

func main() {
	godotenv.Load(".env")

	f, err := os.Open("prs_links_id_uniq.csv")
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

	control := make(chan bool)

	limitCh := make(chan int, 5000)
	rateCh := make(chan int, 5)
	go getRemainingLimit(limitCh)
	go getRate(rateCh)

	f, err = os.OpenFile("data/files.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	writer.Write([]string{
		"comment_id",
		"diff_hunk",
		"filename",
		"blob_url",
		"raw_url"})

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
		prUrl := line[0]
		commentUrl := createCommentUrl(prUrl, line[1])

		go getFileUrl(prUrl, commentUrl, limitCh, rateCh, control, writer, &wg)
		<- control
	}
	writer.Flush()

	wg.Wait()
}
