package main

import (
	"context"
	"fmt"

	"github.com/google/go-github/github"
	"github.com/kr/pretty"
	"golang.org/x/oauth2"
)

func main() {
	githubToken := "11ddc8caef2a0a84649dcf3b6ca2db4f4b1d9b0b"
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	fs, err := githubfs.newGithubfs(client, "darksidergod", "githubfs-test", "master")
	if err != nil {
		panic(err)
	}
	//info, _ := afero.ReadDir(fs, "/")
	//err = fs.Remove("/base.yaml")
	//data, _ := afero.ReadFile(fs, "/core.yaml")
	//os.Stdout.Write(data)
	//err = fs.RemoveAll("/channel-artifacts")
	err = fs.Rename("/configtx.txt", "/configtx.yaml")
	fmt.Printf("%# v", pretty.Formatter(err))
}
