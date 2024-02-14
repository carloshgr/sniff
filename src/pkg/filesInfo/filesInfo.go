package filesInfo

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	// "io"
	"log"
	// "math"
	// "os"
	"strings"
	"sync"
	// "time"

	"github.com/go-resty/resty/v2"
	// "github.com/joho/godotenv"

	"sniff/src/pkg/requestsUtils"
	"sniff/src/pkg/utils"
)

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
			resp = requestsUtils.SendRequest(client, queryParams, url)
	
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
				if utils.IsInList(result["message"].(string), errorMessages) {
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
			resp = requestsUtils.SendRequest(client, queryParams, url)
	
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
				if utils.IsInList(result["message"].(string), errorMessages) {
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

func GetFileUrl(prId string, prUrl string, commentUrl string, limitCh chan int, rateCh chan int, control chan bool, writer *csv.Writer, wg *sync.WaitGroup, logger *log.Logger) {
	commentCh := make(chan map[string]interface{})
	filesCh := make(chan []interface{})
	done := make(chan bool)
	dataCh := make(chan []string)

	filesUrl := fmt.Sprintf("%s/files", prUrl)
	go getCommentRequest(commentUrl, commentCh, limitCh, rateCh, logger)
	go getFilesRequest(filesUrl, filesCh, limitCh, rateCh, logger)
	go filterFiles(prId, filesCh, commentCh, dataCh, logger)
	go utils.WriteComments(writer, dataCh, done)

	<-done
	control <- true
	wg.Done()
}
