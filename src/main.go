package main

import (
	"os"
	"fmt"
	"context"
	"golang.org/x/oauth2"
	"github.com/joho/godotenv"
	graphql "github.com/hasura/go-graphql-client"
)


func main() {
	var query struct {
		Viewer struct {
			Login string
			CreatedAt string
		}
	}
	
	godotenv.Load(".env")

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GRAPHQL_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	client := graphql.NewClient("https://api.github.com/graphql", httpClient)

	err := client.Query(context.Background(), &query, nil)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(query)
}