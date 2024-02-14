package main

import (
	// "encoding/csv"
	// "encoding/json"
	"fmt"
	"io"
	"log"
	// "math"
	"os"
	"strings"
	"sync"
	// "time"

	// "github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
	"sniff/src/pkg/filesInfo"
	"sniff/src/pkg/requestsUtils"
	"sniff/src/pkg/utils"
)

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

	inputFile, csvReader := utils.CreateReader("prs_links_id_uniq.csv")
	defer inputFile.Close()

	os.RemoveAll("./data")
	os.Mkdir("./data", os.ModePerm)

	control := make(chan bool)

	limitCh := make(chan int, 5000)
	rateCh := make(chan int, 5)
	go requestsUtils.GetRemainingLimit(limitCh)
	go requestsUtils.GetRate(rateCh)

	logFile, logger := utils.CreateLogger("data/errorLog.txt")
	defer logFile.Close()

	outputFile, writer := utils.CreateWriter("data/files.csv",
		[]string{
		"pr_id",
		"comment_id",
		"diff_hunk",
		"filename",
		"blob_url",
		"raw_url"})
	defer outputFile.Close()

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

		go filesInfo.GetFileUrl(prId, prUrl, commentUrl, limitCh, rateCh, control, writer, &wg, logger)
		<- control
	}
	writer.Flush()

	wg.Wait()
}
