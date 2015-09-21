package main

import (
    "fmt"
    "math/rand"
    "os"
    "regexp"
    "strings"
    "time"

    "github.com/nlopes/slack"
    "github.com/pelletier/go-toml"
    "gopkg.in/mgo.v2"
    "gopkg.in/mgo.v2/bson"
)

// All the fields of the slack message plus one for id.
type MessageWithId struct {
    Id          bson.ObjectId `json:"id" bson:"_id,omitempty"`
    User        string       `json:"user,omitempty"`
    Text        string       `json:"text,omitempty"`
    Timestamp   string       `json:"ts,omitempty"`
}

// GetUserByName returns a user given a user name
func getUserForName(userName string, info slack.Info) *slack.User {
	for _, user := range info.Users {
		if strings.ToLower(user.Name) == strings.ToLower(userName) {
			return &user
		}
	}
	return nil
}


func main() {
    var info slack.Info
    quothPattern := regexp.MustCompile(`^(?i:quoth(?:\s+(\S+))?)`)
    deletePattern := regexp.MustCompile(`^(?i:forget ([A-Za-z0-9]{24}))`)
    bracketRemovalPattern := regexp.MustCompile(`<(.*?)>`)

    rand.Seed(time.Now().UnixNano())
    // Read config.toml
    config, err := toml.LoadFile("config.toml")
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error opening config file. Did you remember to `cp config.toml.example config.toml`?\n", err.Error())
        os.Exit(1)
    }

    apiToken := config.Get("slack-quoter.api_token").(string)
    channelKey := config.Get("slack-quoter.channel_key").(string)
    mongoDBServer := config.Get("slack-quoter.mongodb_server").(string)

    session, err := mgo.Dial(mongoDBServer)
    if err != nil {
        panic(err)
    }
    defer session.Close()
    col := session.DB("slack").C("quotes")

	api := slack.New(apiToken)
	api.SetDebug(true)

    postMessageParams := slack.NewPostMessageParameters()
    postMessageParams.AsUser = true
    postMessageParams.UnfurlLinks = true

	rtm := api.NewRTM()
	go rtm.ManageConnection()

Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			fmt.Print("Event Received: ")
			switch ev := msg.Data.(type) {
			case *slack.HelloEvent:
				// Ignore hello

			case *slack.ConnectedEvent:
				info = *(ev.Info)
				fmt.Println("Connection counter:", ev.ConnectionCount)
				// Replace #general with your Channel ID

			case *slack.MessageEvent:
			    var quote MessageWithId
			    if ev.Channel != channelKey {
                    continue
			    }

                if matches := quothPattern.FindStringSubmatch(ev.Text); matches != nil {
                    // Ignore bot users.
                    requester := info.GetUserByID(ev.User)
                    if requester != nil && requester.IsBot {
                        continue
                    }
    	            var user *slack.User
    			    if matches[1] != "" {
    			        user = getUserForName(matches[1], info)
    			    }
    			    var query *mgo.Query
    			    if user != nil {
    			        query = col.Find(bson.M{"user": user.ID})
    			    } else {
    			        query = col.Find(nil)
    			    }
    			    count, err := query.Count()
    			    if err != nil {
    		            fmt.Println("Error gettings count: ", err)
                        continue
    		        }
    		        if count == 0 {
                        rtm.SendMessage(rtm.NewOutgoingMessage("No quotes. Consider better posting.", channelKey))
    		            continue
    		        }
    		        query.Skip(rand.Intn(count)).One(&quote)

                    // We have a random quote now. Print to the channel.
                    if user == nil {
                        user = info.GetUserByID(quote.User)
                    }
                    var name string
                    if user != nil {
                        name = user.Name
                    } else {
                        name = "UNKNOWN"
                    }
                    quoteText := bracketRemovalPattern.ReplaceAllString(quote.Text, "$1")
                    api.PostMessage(channelKey, fmt.Sprintf("Quoth %s:\t\t\t\t\t\t(%s)\n%s", name, quote.Id.Hex(), quoteText), postMessageParams)
                } else if matches := deletePattern.FindStringSubmatch(ev.Text); matches != nil {
                    err := col.RemoveId(bson.ObjectIdHex(matches[1]))
                    if err == mgo.ErrNotFound {
                        rtm.SendMessage(rtm.NewOutgoingMessage(fmt.Sprintf("What %s? No %ss here.", matches[1], matches[1]), channelKey))
                    } else {
                        rtm.SendMessage(rtm.NewOutgoingMessage("Beleted " + matches[1] + ".", channelKey))
                    }
                }

			case *slack.PresenceChangeEvent:
				fmt.Printf("Presence Change: %v\n", ev)

			case *slack.LatencyReport:
				fmt.Printf("Current latency: %v\n", ev.Value)

			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())

			case *slack.InvalidAuthEvent:
				fmt.Printf("Invalid credentials")
				break Loop

			case *slack.StarAddedEvent:
			    if ev.Item.Type != "message" {
			        continue
			    }
			    col.Upsert(ev.Item.Message.Msg, ev.Item.Message.Msg)

			default:
				// Ignore other events..
				// fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}
}
