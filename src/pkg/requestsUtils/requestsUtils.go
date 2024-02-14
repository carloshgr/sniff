package requestsUtils

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

func SendRequest(client *resty.Client, queryParams map[string]string, url string) *resty.Response {
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

func GetRate(rateCh chan int) {
	sleepTime := time.Second

	for {
		rateCh <- 1
		time.Sleep(sleepTime)
	}
}

func GetRemainingLimit(limitCh chan int) {
	client := resty.New()
	url := "https://api.github.com/rate_limit"

	var body map[string]interface{}
	for {
		resp := SendRequest(client, make(map[string]string), url)

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
