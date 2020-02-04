package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	marta "github.com/CatOrganization/gomarta"
	flags "github.com/jessevdk/go-flags"

	"go.uber.org/zap"
)

type options struct {
	MartaAPIKey       string `long:"marta-api-key" env:"MARTA_API_KEY" description:"marta api key" required:"true"`
	PollTimeInSeconds int    `long:"poll-time-in-seconds" env:"POLL_TIME_IN_SECONDS" description:"time to poll marta api every second" required:"true"`
	WebhookURL        string `long:"webhook-url" env:"WEBHOOK_URL" description:"slack webhook url" required:"true"`

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

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	martaC := marta.NewDefaultClient(opts.MartaAPIKey)

	httpC := &http.Client{}
	pollTime := time.Duration(opts.PollTimeInSeconds) * time.Second

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			log.Print("getting trains")
			trains, err := martaC.GetTrains()
			if err != nil {
				log.Print(err.Error())
				continue
			}
			for _, train := range trains {
				if train.WaitingTime == "Boarding" {
					log.Print("train is boarding")
					err = SendSlackMessage(ctx, httpC, opts.WebhookURL, train)
					if err != nil {
						log.Print(err.Error())
					}
				}
			}

			time.Sleep(pollTime)
		}
	}()

	select {
	case <-quit:
		cancel()
		logger.Info("interrupt signal received")
		logger.Info("shutting down...")
	}
}

type SlackMessage struct {
	Text string `json:"text"`
}

func SendSlackMessage(ctx context.Context, httpC *http.Client, webhookURL string, train marta.Train) error {
	b, err := json.Marshal(SlackMessage{fmt.Sprint(train.Station, " ", train.Direction, " is now boarding")})
	if err != nil {
		return err
	}
	log.Print("sending message " + webhookURL)
	log.Print(fmt.Sprintf("%s", b))
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(b))
	log.Print(fmt.Sprintf("%s", resp.Body))
	return err
}
