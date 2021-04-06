package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"go.temporal.io/sdk/client"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

type TransmitService struct {
	c         client.Client
	done      chan interface{}
	callbacks map[time.Time]string
}

var _ Server = (*TransmitService)(nil)

func NewTransmitService(c client.Client) *TransmitService {
	return &TransmitService{
		c:         c,
		done:      make(chan interface{}),
		callbacks: make(map[time.Time]string),
	}
}

func (r *TransmitService) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	log.Println("ServeHTTP")

	// Do nothing if we failed the callback roll
	if rand.Intn(100) >= TransmitServiceCallbackChance {
		log.Println("ServeHTTP: failed callback roll")
		w.WriteHeader(http.StatusOK)
		return
	}
	log.Println("ServeHTTP: succeeded on callback roll")

	// Roll a random delay
	delay := time.Now().Add(time.Duration(rand.Intn(TransmitServiceTimeoutSecs)) * time.Second)
	log.Println("ServeHTTP: delay", delay)

	if err := request.ParseForm(); err != nil {
		log.Println("invalid form")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Println("ServeHTTP: token", request.Form.Get(ParamTaskToken))
	token := request.Form.Get(ParamTaskToken)
	r.callbacks[delay] = token
	w.WriteHeader(http.StatusOK)
}

func (r *TransmitService) String() string {
	return "TransmitService"
}

func (r *TransmitService) process() {
	var toDelete []time.Time
	for t, token := range r.callbacks {
		if time.Now().After(t) {
			tokenBytes, _ := base64.RawURLEncoding.DecodeString(token)
			err := r.c.CompleteActivity(context.Background(), tokenBytes, TransmitOut{Delivered: true}, nil)
			if err != nil {
				log.Printf("activity completion error: %v\n", err)
			}
			toDelete = append(toDelete, t)
		}
	}
	for _, t := range toDelete {
		delete(r.callbacks, t)
	}
}

func SubmitToTransmitService(token string) error {
	data := url.Values{}
	data.Set(ParamTaskToken, token)
	_, err := http.PostForm(fmt.Sprintf("http://localhost:%d/%s", PortTransmitService, PathRouting), data)
	if err != nil {
		return err
	}
	return nil
}
