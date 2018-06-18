// Telegram notification bot
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"gopkg.in/telegram-bot-api.v4"
)

//var clientPath = filepath.Join(file.UserHome(), ".skywire", "manager", "clients.json")
var clientPath = "./test.json"

// clientConnection is a structure that represents the JSON file structure
// of the clients.json file (from Skywire project)
type clientConnection struct {
	Label   string `json:"label"`
	NodeKey string `json:"nodeKey"`
	AppKey  string `json:"appKey"`
	Count   int    `json:"count"`
}

// Defines an in-memory slice (dynamic array) based on the ClientConnection struct
type clientConnectionSlice []clientConnection

// Determines if the specified ClientConnection exists within the clientConnectionSlice
func (c clientConnectionSlice) Exist(rf clientConnection) bool {
	for _, v := range c {
		if v.AppKey == rf.AppKey && v.NodeKey == rf.NodeKey {
			return true
		}
	}
	return false
}

// Reads the physical Skywire Clients.JSON file into an in-memory structure
func readClientConnectionConfig() (cfs map[string]clientConnectionSlice, err error) {
	fb, err := ioutil.ReadFile(clientPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfs = nil
			err = nil
			return
		} else {
			return
		}
	}
	cfs = make(map[string]clientConnectionSlice)
	err = json.Unmarshal(fb, &cfs)
	if err != nil {
		return
	}
	return
}

// getClientConnectionListString will iterate over the ClientConnectionConfig JSON
// file and return a formatted string for all Clients and their Nodes
func getClientConnectionListString() string {
	var clientsb strings.Builder

	// Read the Client Connection Config (JSON) into ccc
	ccc, err := readClientConnectionConfig()
	if err == nil {
		// Iterate ccc reading the Keys (k)
		for k := range ccc {
			// Output to our string builder the current Client Type (from K)
			// Add an newline if this isnt the first itteration
			if clientsb.String() != "" {
				clientsb.WriteString("\n")
			}
			clientsb.WriteString(fmt.Sprintf("ClientType: [%s]\n", k))
			// Iterate all Nodes in the current client type (ccc[k])
			for _, b := range ccc[k] {
				// Output the details for each node of this client type
				clientsb.WriteString(fmt.Sprintf("Label:   %s\n", b.Label))
				clientsb.WriteString(fmt.Sprintf("NodeKey: %s\n", b.NodeKey))
				clientsb.WriteString(fmt.Sprintf("AppKey:  %s\n", b.AppKey))
				clientsb.WriteString("\n")
			}
		}
	}
	log.Debugln(clientsb.String())
	// Return the built string
	return clientsb.String()
}

// The BotConfig struct is used to store run-time configuration
// information for the bot application.
type botConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   int64  `json:"chat_id"`
	Locked   bool   `json:"locked"`
	BotDebug bool   `json:"botdebug"`
}

var (
	config      botConfig
	telegramBot tgbotapi.BotAPI
)

// parseFlags parses command line flags and populates the run-time applicaton configuration
func parseFlags() {
	flag.StringVar(&config.BotToken, "bottoken", "", "telegram bot token (provided by the @BotFather")
	flag.BoolVar(&config.BotDebug, "botdebug", false, "Bot API debugging")
	flag.Parse()

	log.Debugf("Parameter: bottoken = %s", config.BotToken)
	log.Debugf("Parameter: botdebug = %v", config.BotDebug)
}

// chatSetup checks
func chatSetup() bool {
	return (config.ChatID != -1)
}

// watchFile will watch the file specified by filename
func watchFile(fileEventMsg chan<- string, stopEvent chan bool, filename string) {
	log.Debugf("[FWM] Seting up monitoring on file: %s", filename)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Panic(err)
	}
	defer watcher.Close()
	defer log.Infof("[FWM] Stop watching file: %s", filename)

	err = watcher.Add(filename)
	if err != nil {
		log.Panic(err)
	}

	log.Infof("[FWM] Start watching file: %s", filename)
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Debugf("[FWM] (%s) handling event  [%s]\n", event.Name, event.Op)
				msgText := getClientConnectionListString()
				log.Debug(msgText)
				fileEventMsg <- msgText
			} else {
				log.Debugf("[FWM] (%s) ignorning event [%s]\n", event.Name, event.Op)
			}
		case err := <-watcher.Errors:
			log.Errorln("File watcher error:", err)
		case stop := <-stopEvent:
			if stop {
				log.Debugln("[FWM] Stop event recieved.")
				break
			}
		}
	}
}

// Create new telegram bot using the bot token passed on the cmd line
func startTelegramBot(botwg *sync.WaitGroup) {
	var watcherRunning = false

	telegramBot, err := tgbotapi.NewBotAPI(config.BotToken)
	if err != nil {
		log.Panic(err)
	}
	defer log.Info("[BOT] Telegram Bot finished.")

	monitorMsgEvent := make(chan string)
	defer close(monitorMsgEvent)
	monitorStopEvent := make(chan bool, 1)
	defer close(monitorStopEvent)

	telegramBot.Debug = config.BotDebug
	log.Infof("[BOT] Telegram Bot connected and authorised on account %s", telegramBot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := telegramBot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil {
			log.Debug("[BOT] Ignoring empty message.")
			continue
		}

		config.ChatID = update.Message.Chat.ID
		log.Debugf("[BOT] Message recieved from ChatID: %v", config.ChatID)

		if update.Message.IsCommand() {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
			log.Debugf("[BOT] Recieved Command: %s", update.Message.Command())
			switch update.Message.Command() {
			case "help":
				msg.Text = "type /about or /status or /chatid or /start or /stop."
			case "about":
				msg.Text = "Skywire Manager TelegraTelegram Monitoring Bot\n"
				msg.Text = msg.Text + "By @BigOokie\n"
				msg.Text = msg.Text + "GitHub: https://github.com/BigOokie/skywire-telegram-notify-bot"
			case "status":
				msg.Text = "I'm fine. Still running :)"
			case "chatid":
				msg.Text = fmt.Sprintf("ChatID: %v", update.Message.Chat.ID)
			case "start":
				if watcherRunning {
					msg.Text = "Monitor start has already been requested."
					log.Debugln(msg.Text)
				} else {
					watcherRunning = true
					msg.Text = "Monitor start requested."
					// Start watching the Skywire Monitors clients.json file
					go watchFile(monitorMsgEvent, monitorStopEvent, "./test.json")
				}
			case "stop":
				msg.Text = "Monitor stop requested."
				monitorStopEvent <- true
				watcherRunning = false
			default:
				msg.Text = "Sorry. I don't know that command."
			}
			log.Debugln("[BOT] Start Response]")
			log.Debugln(msg.Text)
			log.Debugln("[BOT] End Response]")
			telegramBot.Send(msg)
		}
	}
	// Signal the Bot has finished
	botwg.Done()
}

func main() {
	log.SetFormatter(&log.TextFormatter{})
	log.SetLevel(log.DebugLevel)
	log.Infoln("Starting Skywire Telegram Notification Bot App.")
	defer log.Infoln("Stopping Skywire Telegram Notification Bot App. Bye.")
	parseFlags()

	// Setup a waitgroup to sync and wait for the Telegram Bot to end.
	var botwg sync.WaitGroup
	botwg.Add(1)
	// Start the telegram bot
	go startTelegramBot(&botwg)
	botwg.Wait()

	// Start watching the Skywire Monitors clients.json file
	//go watchFile(msgChannel, "./test.json")

	//for {
	//	txt := <-msgChannel
	//	log.Debugf("Event: %v", txt)
	//}

	/*
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates, err := bot.GetUpdatesChan(u)
		for update := range updates {
			if update.Message == nil {
				log.Warn("Ignoring empty message.")
				continue
			}

			log.Infof("Message recieved from ChatID: %v", update.Message.Chat.ID)

			if allowedToRespondToChat(update.Message.Chat.ID) {
				// Allowed to respond to this Chat ID
				if update.Message.IsCommand() {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
					log.Debugf("Command: %s", update.Message.Command())
					switch update.Message.Command() {
					case "help":
						msg.Text = "type /sayhi or /status or /chatid or /lock or /unlock."
					case "sayhi":
						msg.Text = "Hi :)"
					case "status":
						msg.Text = "I'm ok."
					case "chatid":
						msg.Text = fmt.Sprintf("ChatID: %v", update.Message.Chat.ID)
					case "lock":
						if config.ChatID == -1 {
							config.ChatID = update.Message.Chat.ID
							msg.Text = fmt.Sprintf("Locked to current ChatID [%v]", update.Message.Chat.ID)
						} else {
							msg.Text = "Already locked."
						}
					case "unlock":
						if config.ChatID == -1 {
							msg.Text = "Already unlocked."
						} else {
							config.ChatID = -1
							msg.Text = "Unlocked."
						}
					default:
						msg.Text = "Sorry. I don't know that command."
					}
					bot.Send(msg)
				}
			} else {
				log.Warnf("Chat is locked to ID [%v]. Not allowed to respond to Chat ID [%v]", config.ChatID, update.Message.Chat.ID)
			}
		}
	*/
}
