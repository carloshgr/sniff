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

func isInList(target string, list []string) bool {
	for _, element := range list {
		if element == target {
			return true
		}
	}
	return false
}

func getFilesRequest(url string, filesCh chan []interface{}, limitCh chan int, rateCh chan int, logger *log.Logger) {
	client := resty.New()

	var resp *resty.Response
	var completeResp []interface{}
	var files interface{}

	finished := false
	page := 1

	queryParams := map[string]string{
		"per_page": "100",
		"page": "1",
	}

	errorMessages := []string{
		"Resource protected by organization SAML enforcement. You must grant your Personal Access token access to this organization.",
		"Not Found",
	}

	for !finished {
		for {
			<-limitCh
			<-rateCh
			resp = sendRequest(client, queryParams, url)
	
			if resp.IsSuccess() {
				log.Printf("Request, %s, %d, success", url, page)

				json.Unmarshal(resp.Body(), &files)
				files, _ := files.([]interface{})
				finished = len(files) == 0
				if !finished {
					completeResp = append(completeResp, files...)
				}

				break
			} else {
				json.Unmarshal(resp.Body(), &files)
				result, _ := files.(map[string]interface{})

				logger.Printf("Request, %s, %d, failed, %s", url, page, resp.Body())
				if isInList(result["message"].(string), errorMessages) {
					finished = true
					break
				}
			}
		}
		page = page + 1
		queryParams["page"] = fmt.Sprintf("%d", page)
	}

	filesCh <- completeResp
	close(filesCh)
}

func getCommentRequest(url string, commentCh chan map[string]interface{}, limitCh chan int, rateCh chan int, logger *log.Logger) {
	client := resty.New()

	var resp *resty.Response
	var comment interface{}
	var parsedComment map[string]interface{}
	finished := false

	errorMessages := []string{
		"Resource protected by organization SAML enforcement. You must grant your Personal Access token access to this organization.",
		"Not Found",
	}

	queryParams := map[string]string{}
	
	for !finished {
		for {
			<-limitCh
			<-rateCh
			resp = sendRequest(client, queryParams, url)
	
			if resp.IsSuccess() {
				log.Printf("Request, %s, success", url)

				json.Unmarshal(resp.Body(), &comment)
				comment, _ := comment.(map[string]interface{})
				parsedComment = comment
				finished = true

				break
			} else {
				json.Unmarshal(resp.Body(), &comment)
				result, _ := comment.(map[string]interface{})

				logger.Printf("Request, %s, failed, %s", url, resp.Body())
				if isInList(result["message"].(string), errorMessages) {
					finished = true
					parsedComment = result
					break
				}
			}
		}
	}

	commentCh <- parsedComment
	close(commentCh)
}

func getPRUrlFromComment(comment map[string]interface{}) string {
	_, exists := comment["message"]
	if exists {
		return ""
	} else  {
		pullRequestObj := comment["_links"].(map[string]interface{})["pull_request"]
		prLink := pullRequestObj.(map[string]interface{})["href"]
	
		return prLink.(string)
	}
}

func filterFiles(prId string, filesCh chan []interface{}, commentCh chan map[string]interface{}, dataCh chan []string, logger *log.Logger) {
	comment := <- commentCh
	files := <- filesCh

	replacer := strings.NewReplacer("\r", "", "\n", "")
	var data []string
	var filenames []string
	var ref string

	ref = getPRUrlFromComment(comment)
	for _, file := range files {
		fileMap, _ := file.(map[string]interface{})

		filenames = append(filenames, fileMap["filename"].(string))
		if fileMap["filename"] == comment["path"] {
			data = append(data, prId)
			data = append(data, fmt.Sprintf("%d", int(comment["id"].(float64))))
			data = append(data, replacer.Replace(comment["diff_hunk"].(string)))
			data = append(data, fileMap["filename"].(string))
			data = append(data, fileMap["blob_url"].(string))
			data = append(data, fileMap["raw_url"].(string))
		}
	}
	if len(data) == 0 {
		logger.Printf("PR URL: %s | Comment path %s not in PR files %s", ref, comment["path"], filenames)
	}
	dataCh <- data
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

func getFileUrl(prId string, prUrl string, commentUrl string, limitCh chan int, rateCh chan int, control chan bool, writer *csv.Writer, wg *sync.WaitGroup, logger *log.Logger) {
	commentCh := make(chan map[string]interface{})
	filesCh := make(chan []interface{})
	done := make(chan bool)
	dataCh := make(chan []string)

	filesUrl := fmt.Sprintf("%s/files", prUrl)
	go getCommentRequest(commentUrl, commentCh, limitCh, rateCh, logger)
	go getFilesRequest(filesUrl, filesCh, limitCh, rateCh, logger)
	go filterFiles(prId, filesCh, commentCh, dataCh, logger)
	go writeComments(writer, dataCh, done)

	<-done
	control <- true
	wg.Done()
}

func createCommentUrl(prUrl string, commentId string) string {
	data := strings.Split(prUrl, "/")
	return fmt.Sprintf("%s/%s/comments/%s", data[0], strings.Join(data[1:len(data)-1], "/"), commentId)
}

func getPullRequestIdFromRequest(prUrl string) string {
	data := strings.Split(prUrl, "/")
	return data[len(data)-1]
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

	logFile, err := os.OpenFile("data/errorLog.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Error opening log file:", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "ErrorLogger: ", log.Ldate|log.Ltime|log.Lshortfile)

	writer := csv.NewWriter(f)
	writer.Write([]string{
		"pr_id",
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
		prId := getPullRequestIdFromRequest(prUrl)

		go getFileUrl(prId, prUrl, commentUrl, limitCh, rateCh, control, writer, &wg, logger)
		<- control
	}
	writer.Flush()

	wg.Wait()
}
