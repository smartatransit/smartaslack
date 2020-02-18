package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"smartatransit/smartaslack/pkg/slack"

	marta "github.com/CatOrganization/gomarta"
	flags "github.com/jessevdk/go-flags"

	"go.uber.org/zap"
)

type options struct {
	MartaAPIKey     string `long:"marta-api-key" env:"MARTA_API_KEY" description:"marta api key" required:"true"`
	SlackSigningKey string `long:"slack-signing-key" env:"SLACK_SIGNING_KEY" description:"slack signing key" required:"true"`

	Debug bool `long:"debug" env:"DEBUG" description:"enabled debug logging"`
}

func main() {
	fmt.Println("Starting smartaslack")
	var opts options
	_, err := flags.Parse(&opts)
	if err != nil {
		log.Fatal(err)
	}

	var logger *zap.Logger
	if opts.Debug {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer func() {
		_ = logger.Sync() // flushes buffer, if any
	}()
	sv := slack.Verifier{
		Secret:  opts.SlackSigningKey,
		Version: "v0",
		Logger:  logger,
	}
	martaC := marta.NewDefaultClient(opts.MartaAPIKey)
	mux := http.NewServeMux()
	fah := &findArrivalHandler{martaC, logger, sv, opts.Debug}
	mux.Handle("/find-arrival", fah)

	err = http.ListenAndServe(":3000", mux)
	log.Fatal(err)
}

type findArrivalHandler struct {
	martaC *marta.Client
	logger *zap.Logger
	sv     slack.Verifier
	debug  bool
}

type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Block struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

type SlackMessage struct {
	Blocks []Block `json:"blocks"`
}

func (th *findArrivalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rawURL, err := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(rawURL))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !th.debug {
		validatedSignature := th.sv.IsValid(
			fmt.Sprintf("%s", rawURL),
			r.Header.Get("X-Slack-Request-Timestamp"),
			r.Header.Get("X-Slack-Signature"),
		)
		if !validatedSignature {
			return
		}
	}
	trains, err := th.martaC.GetTrains()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	th.logger.Debug(fmt.Sprintf("%v", r.Form))
	filteredTrains := filterTrainsByStation(trains, r.FormValue("text"))
	w.Header().Add("Content-Type", "application/json")
	if len(filteredTrains) == 0 {
		http.Error(w, "No trains found with that station name", http.StatusNotFound)
		return
	}
	blocks := buildSlackMessage(filteredTrains)
	b, err := json.Marshal(&blocks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func filterTrainsByStation(trains []marta.Train, station string) (filtered []marta.Train) {
	for _, train := range trains {
		if train.Station == station {
			filtered = append(filtered, train)
		}
	}
	return
}

func buildSlackMessage(trains []marta.Train) (msg SlackMessage) {
	for _, train := range trains {
		msg.Blocks = append(msg.Blocks, buildBlock(train))
	}
	return
}

func buildBlock(train marta.Train) (block Block) {
	return Block{
		Type: "section",
		Text: Text{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*:train2:%s:train2:*\\n%s arriving in %s", train.Station, train.Direction, train.WaitingTime),
		},
	}
}
