package main

import (
	"context"
	"log"

	"github.com/darksidergod/githubfs-test"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func main() {
	githubToken := "d50dbcdaa358e902625a907e5502fe21ab09d915"
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	fs, err := githubfs.NewGithubfs(client, "darksidergod", "githubfs-test", "master")
	if err != nil {
		panic(err)
	}

	_, err = fs.Create("demo/foobar")
	if err != nil {
		log.Fatal(err)
	}
	//fmt.Printf("%# v", pretty.Formatter(fs))
}
