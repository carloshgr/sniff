package main

import (
	// "encoding/csv"
	// "encoding/json"
	"fmt"
	"io"
	"log"
	"bufio"
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

// type Params struct {
// 	logFile string
// 	inputFile string
// 	outputFile string
// 	outputColumns []string
// }

// type InputData struct {
// 	line []string
// 	writer *csv.Writer
// 	outputFile *os.File
// }

func createCommentUrl(prUrl string, commentId string) string {
	data := strings.Split(prUrl, "/")
	return fmt.Sprintf("%s/%s/comments/%s", data[0], strings.Join(data[1:len(data)-1], "/"), commentId)
}

func getPullRequestIdFromRequest(prUrl string) string {
	data := strings.Split(prUrl, "/")
	return data[len(data)-1]
}

func formatInputData(choice string, line []string, params map[string]utils.Params) utils.InputData {
	var input utils.InputData 
	if choice == "1" {
		outputFile, writer := utils.CreateWriter(params[choice].OutputFile, params[choice].OutputColumns)
		input = utils.InputData{Line: line, Writer: writer, OutputFile: outputFile}
	} else {
		input = utils.InputData{Line: line, Writer: nil, OutputFile: nil}
	}
	return input
}

func main() {
	godotenv.Load(".env")

	params := map[string]utils.Params{
		"1": utils.Params{
			LogFile: "getFilesUrls.log",
			InputFile: "prs_links_id_uniq.csv",
			OutputFile: "files_urls.csv",
			OutputColumns: []string{
				"pr_id",
				"comment_id",
				"diff_hunk",
				"filename",
				"blob_url",
				"raw_url",
			},
		},
		"2": utils.Params{
			LogFile: "getFiles.log",
			InputFile: "files_urls.csv",
			OutputFile: "",
			OutputColumns: []string{},
		},
	}

	reader := bufio.NewReader(os.Stdin)

    fmt.Print("Enter what script you want to execute\n[1] - Get urls to download files\n[2] - Get files using the urls\nType your choice: ")
    choice, _ := reader.ReadString('\n')

	control := make(chan bool)

	limitCh := make(chan int, 5000)
	rateCh := make(chan int, 5)
	go requestsUtils.GetRemainingLimit(limitCh)
	go requestsUtils.GetRate(rateCh)

	inputFile, csvReader := utils.CreateReader("prs_links_id_uniq.csv")
	defer inputFile.Close()

	os.RemoveAll("./data")
	os.Mkdir("./data", os.ModePerm)

	logFile, logger := utils.CreateLogger("data/errorLog.txt")
	defer logFile.Close()

	// outputFile, writer := utils.CreateWriter("data/files.csv",
	// 	[]string{
	// 	"pr_id",
	// 	"comment_id",
	// 	"diff_hunk",
	// 	"filename",
	// 	"blob_url",
	// 	"raw_url"})
	// defer outputFile.Close()

	// #####################################################
	// extract creation of the writer to fix the problem
	// #####################################################

	var wg sync.WaitGroup
	for {
		line, err := csvReader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatal(err)
		}

		input := formatInputData(choice, line, params)

		wg.Add(1)
		// go getFile(line, limitCh, rateCh, control, &wg, logger)
		go filesInfo.GetFileUrl(input, limitCh, rateCh, control, &wg, logger)
		<- control
	}
	writer.Flush()

	wg.Wait()
}
