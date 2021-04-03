package main

import (
	"context"
	"flag"
	"fmt"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var dataRoutingTaskQueue = "DataRoutingTaskQueue"

var _ http.Handler = (*RoutingService)(nil)

func doWorker() {
	c, err := client.NewClient(client.Options{})
	if err != nil {
		log.Fatal(err)
	}
	w := worker.New(c, dataRoutingTaskQueue, worker.Options{})
	w.RegisterActivity(GetRouteActivity)
	w.RegisterActivity(TransmitActivity)
	w.RegisterWorkflow(DataRoutingWorkflow)

	routingService := NewRoutingService(c)
	mux := http.NewServeMux()
	mux.Handle("/submit", routingService)
	httpd := &http.Server{
		Addr:    fmt.Sprintf(":%d", PortRoutingService),
		Handler: mux,
	}

	ms := NewMultiServer(NewHTTPServer(httpd, routingService), NewTemporalWorker(w), routingService)
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		ms.Stop()
	}()

	ms.Start()

}

func doSubmit() {
	c, err := client.NewClient(client.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	dataRoutingIn := DataRoutingIn{
		// Behaviors such as the number of retries can be specified workflow input
		GetRouteMaxAttempts:       GetRouteAttempsCount,
		TransmitPacketMaxAttempts: TransmitPacketMaxAttempts,

		Packet: Packet{
			ID:          "00000000-0000-0000-0000-000000000000",
			Source:      "+017771110000",
			Destination: "+016660002222",
			Contents:    []byte("Hello"),
			Status:      StatusNoRoute,
		},
	}

	workflowOptions := client.StartWorkflowOptions{
		// As a norm, the workflow id contains an ID for the business entity
		ID: fmt.Sprintf("DataRoutingWorkflow-%s", dataRoutingIn.Packet.ID),

		// Include a version number in the TaskQueue is one way to run multiple worker code concurrently
		TaskQueue: "DataRoutingTaskQueueV1",
	}
	we, err := c.ExecuteWorkflow(context.Background(), workflowOptions, DataRoutingWorkflow, dataRoutingIn)
	if err != nil {
		log.Fatal(err)
	}
	var dataRoutingOut DataRoutingOut
	if err := we.Get(context.Background(), &dataRoutingOut); err != nil {
		log.Fatal(err)
	}

	log.Println(color(toJson(dataRoutingOut)))
}
func main() {
	var subcommand string
	flag.StringVar(&subcommand, "do", "submit", "either: worker or submit")
	flag.Parse()

	switch subcommand {
	case "worker":
		doWorker()
	case "submit":
		doSubmit()
	default:
		panic("invalid subcommand")
	}
}
