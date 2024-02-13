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

func getFilesRequest(url string, filesCh chan string, limitCh chan int, rateCh chan int, logger *log.Logger) {
	client := resty.New()

	var resp *resty.Response
	var content string
	finished := false

	// errorMessages := []string{
	// 	"Resource protected by organization SAML enforcement. You must grant your Personal Access token access to this organization.",
	// 	"Not Found",
	// }

	queryParams := map[string]string{}
	
	for !finished {
		for {
			<-limitCh
			<-rateCh
			resp = sendRequest(client, queryParams, url)
	
			if resp.IsSuccess() {
				log.Printf("Request, %s, success", url)
				content = fmt.Sprintf("%s", resp.Body())
				finished = true
				break
			} else {
				logger.Printf("Request, %s, failed, %s", url, resp.Body())
				// if isInList(result["message"].(string), errorMessages) {
				// 	finished = true
				// 	parsedComment = result
				// 	break
				// }
			}
		}
	}

	filesCh <- content
	close(filesCh)
}

func getFilenameFromFilepath(filepath string) string {
	data := strings.Split(filepath, "/")
	return data[len(data)-1]
}

func saveFiles(filesCh chan string, prId string, commentId string, filepath string, done chan bool, logger *log.Logger) {
	content := <- filesCh
	filename := getFilenameFromFilepath(filepath)
	destination := fmt.Sprintf("./files/%s_%s_%s", prId, commentId, filename)

	file, err := os.Create(destination)
	if err != nil {
		logger.Printf("Error creating the file %s: %s", filename, err)
		done <- true
		return
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		logger.Printf("Error writing the file %s", filename)
		return
	}

	log.Printf("File %s saved", destination)

	done <- true
}

func getFile(rawUrl string, prId string, commentId string, filename string, limitCh chan int, rateCh chan int, control chan bool, wg *sync.WaitGroup, logger *log.Logger) {
	filesCh := make(chan string)
	done := make(chan bool)

	go getFilesRequest(rawUrl, filesCh, limitCh, rateCh, logger)
	go saveFiles(filesCh, prId, commentId, filename, done, logger)

	<-done
	control <- true
	wg.Done()
}

func main() {
	godotenv.Load(".env")

	f, err := os.Open("./data/files.csv")
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

	os.RemoveAll("./files")
	os.Mkdir("./files", os.ModePerm)

	control := make(chan bool)

	limitCh := make(chan int, 5000)
	rateCh := make(chan int, 5)
	go getRemainingLimit(limitCh)
	go getRate(rateCh)

	logFile, err := os.OpenFile("files/errorLog.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Error opening log file:", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "ErrorLogger: ", log.Ldate|log.Ltime|log.Lshortfile)


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
		prId := line[0]
		commentId := line[1]
		filename := line[3]
		rawUrl := line[5]

		go getFile(rawUrl, prId, commentId, filename, limitCh, rateCh, control, &wg, logger)
		<- control
	}

	wg.Wait()
}
