package main

import (
    "fmt"
    "os"

    "github.com/nlopes/slack"
    "github.com/pelletier/go-toml"
    "gopkg.in/mgo.v2"
)


func main() {
    // Read config.toml
    config, err := toml.LoadFile("config.toml")
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error opening config file. Did you remember to `cp config.toml.example config.toml`?\n", err.Error())
        os.Exit(1)
    }

    apiToken := config.Get("slack-quoter.loader_api_token").(string)
    channelKey := config.Get("slack-quoter.channel_key").(string)
    mongoDBServer := config.Get("slack-quoter.mongodb_server").(string)

    session, err := mgo.Dial(mongoDBServer)
    if err != nil {
        panic(err)
    }
    defer session.Close()
    col := session.DB("slack").C("quotes")

	api := slack.New(apiToken)

    // Get all users.
    users, err := api.GetUsers()
    if err != nil {
        panic(err)
    }

    // Get all starred items.
    starParams := slack.NewStarsParameters()
    for _, user := range users {
        if user.IsBot {
            fmt.Println("BLEEP BLOOP")
            continue
        }
        fmt.Printf("Trying user ID %s with name %s\n", user.ID, user.Name);
        pages := 1
        starParams.User = user.ID
        starParams.Page = 1
        for starParams.Page <= pages {
            stars, paging, err := api.GetStarred(starParams)
            if err != nil {
                panic(err)
            }
            for _, star := range stars {
                if star.Type == "message" {
                    col.Upsert(star.Message.Msg, star.Message.Msg)
                }
            }
            pages = paging.Pages
            starParams.Page += 1
        }
    }

    // Get all pins.
    pinParams := slack.NewListPinsParameters()
    pages := 1
    for pinParams.Page <= pages {
        items, paging, err := api.ListPins(channelKey, pinParams)
        if err != nil {
            panic(err)
        }
        for _, item := range items {
            if item.Type == "message" {
                col.Upsert(item.Message.Msg, item.Message.Msg)
            }
        }
        pages = paging.Pages
        pinParams.Page += 1
    }
}
