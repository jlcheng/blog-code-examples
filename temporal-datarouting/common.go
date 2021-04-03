package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

var (
	// Start a HTTP server to handle "deliver this packet" requests. The server lisens on port 9091, but can be configured by
	// the environment variable "demo_port"
	PortRoutingService = getServerPort(9091, "demo_port")
	// The URL of the "deliver this packet" request is /submit.
	PathRouting = "submit"
	// The URL parameter which describes the origin of a request. This is the task token used to signal the activity
	// whether the delivery succeed or failed.
	ParamTaskToken = "task_token"

	// We try at most three times to look for different route providers.
	TransmitPacketMaxAttempts = 4

	// The remote call to return the next route provider has some chance of failing, configured by the environment variable
	// "error_chance".
	GetRouteErrChance = getErrorChance(10, "error_chance")
	// When a service fails, we pretend it failed due to a network error.
	NetworkError = fmt.Errorf("IO Error: network disconnect")
	// When Temporal retries a failed GetRouteActivity call, it will retry up to two times for a total of three attempts.
	GetRouteAttempsCount = 3
	// The names of the different route providers to choose from.
	PossibleRouteProviders = []string{"RouteA", "RouteB", "RouteC", "RouteD", "RouteE"}

	// The TransmitActivity will timeout after 10 seconds.
	TransmitActivityTimeout = 10 * time.Second

	// The RoutingService has a 90% chance of telling us whether it succeeded or failed.
	RoutingServiceCallbackChance = 90
	// The RoutingService takes up to 15 seconds to make a callback. Note that it might make the callback well after the
	// TransmitActivityTimeout.
	RoutingServiceTimeoutSecs = 15

	// When all route providers have been tried, a packet ends up with a status of NO ROUTE
	StatusNoRoute = "STATUS_NO_ROUTE"
	// If one of the route provider responds with a success within the TransmitActivityTimeout, a packet ends up with a status
	// of DELIVERED
	StatusDelivered = "STATUS_DELIVERED"
)

type (
	Packet struct {
		ID                   string
		Source               string
		Destination          string
		Contents             []byte
		FailedRouteProviders []string
		CurrentRouteProvider string
		Status               string
	}

	DataRoutingIn struct {
		TransmitPacketMaxAttempts int
		GetRouteMaxAttempts       int

		Packet Packet
	}

	DataRoutingOut struct {
		Packet Packet
	}

	GetRouteIn struct {
		Packet Packet
	}

	GetRouteOut struct {
		Empty bool
		Route string
	}

	TransmitIn struct {
		Packet Packet
	}

	TransmitOut struct {
		Delivered bool
	}
)

func color(template string, args ...interface{}) string {
	return fmt.Sprintf("\033[1;33m%s\033[0m", fmt.Sprintf(template, args...))
}

func toJson(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func getErrorChance(defaultErrorChance int, k string) int {
	c, err := strconv.Atoi(os.Getenv(k))
	if err != nil {
		return defaultErrorChance
	}
	if c > 99 {
		c = 99
	}
	if c < 0 {
		c = 0
	}
	return c
}

func getServerPort(defaultPort int, k string) int {
	p, err := strconv.Atoi(os.Getenv(k))
	if err != nil {
		return defaultPort
	}
	return p
}

func GetRoute(tried []string) (GetRouteOut, error) {
	if rand.Intn(100) < GetRouteErrChance {
		return GetRouteOut{}, NetworkError
	}

	t := make(map[string]bool)
	for _, elem := range tried {
		t[elem] = true
	}
	for _, route := range PossibleRouteProviders {
		if !t[route] {
			return GetRouteOut{
				Empty: false,
				Route: route,
			}, nil
		}
	}
	return GetRouteOut{
		Empty: true,
		Route: "",
	}, nil
}
