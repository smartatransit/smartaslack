package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	marta "github.com/CatOrganization/gomarta"
	flags "github.com/jessevdk/go-flags"

	"go.uber.org/zap"
)

type options struct {
	MartaAPIKey string `long:"marta-api-key" env:"MARTA_API_KEY" description:"marta api key" required:"true"`

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

	martaC := marta.NewDefaultClient(opts.MartaAPIKey)
	mux := buildMux(martaC)

	err = http.ListenAndServe(":3000", mux)
	log.Fatal(err)
}

func buildMux(martaC *marta.Client) *http.ServeMux {
	mux := http.NewServeMux()
	fah := &findArrivalHandler{martaC}
	mux.Handle("/find-arrival", fah)
	return mux
}

type findArrivalHandler struct {
	martaC *marta.Client
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

type SlackRequest struct {
	ResponseURL string `json:"response_url"`
	Text        string `json:"text"`
}

func (th *findArrivalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req := SlackRequest{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	trains, err := th.martaC.GetTrains()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filteredTrains := filterTrainsByStation(trains, req.Text)
	w.Header().Add("Content-Type", "application/json")
	blocks := buildSlackMessage(filteredTrains)
	b, err := json.Marshal(&blocks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	return
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
			Text: fmt.Sprintf("*%s*\n%s arriving in %s", train.Station, train.Direction, train.WaitingTime),
		},
	}
	return
}
