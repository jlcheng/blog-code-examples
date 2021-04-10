package main

// See https://pkg.go.dev/go.temporal.io/sdk for documentation on the Activity and Workflow API

import (
	"context"
	"encoding/base64"
	"fmt"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	"time"
)

// GetRouteActivity asks a routing service to find the next available route provider
func GetRouteActivity(ctx context.Context, in GetRouteIn) (GetRouteOut, error) {
	packet := in.Packet
	r, err := GetRoute(packet.FailedRouteProviders)
	if err != nil {
		return GetRouteOut{}, err
	}
	return r, nil
}

// TransmitActivity transmits a packet via the current routing provider
func TransmitActivity(ctx context.Context, in TransmitIn) (TransmitOut, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("TransmitActivity", zap.String("packet_id", in.Packet.ID))
	token := base64.RawURLEncoding.EncodeToString(activity.GetInfo(ctx).TaskToken)
	if err := SubmitToTransmitService(token); err != nil {
		logger.Error("failed to submit data to routing service", zap.Error(err))
		return TransmitOut{}, err
	}
	return TransmitOut{}, activity.ErrResultPending
}

// DataRoutingWorkflow delivers a Packet object to its destination, trying multiple routing providers.
func DataRoutingWorkflow(ctx workflow.Context, in DataRoutingIn) (DataRoutingOut, error) {
	logger := workflow.GetLogger(ctx)
	packet := in.Packet

	// As long as status != Delivered AND we have not tried MaxAttempts transmits, we will execute the core of the workflow.
	for packet.Status != StatusDelivered && len(packet.FailedRouteProviders) < in.TransmitPacketMaxAttempts {
		// This stanza:
		//   1. Configures how the Activity is to be executed.
		//   2. Executes the GetRouteActivity, which calls a fragile network service to get the "next route provider to try".
		//   3. Updates the packet with the "the route provider to try" if previous step succeeds.
		var activityCtx workflow.Context
		var getRouteOut GetRouteOut
		activityCtx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			// ScheduleToStartTimeout - The queue timeout before the activity starts executed.
			// Mandatory: No default.
			ScheduleToStartTimeout: time.Minute,
			// StartToCloseTimeout - The timeout from the start of execution to end of it.
			// Mandatory: No default.
			StartToCloseTimeout: time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Millisecond * 250,
				BackoffCoefficient: 1.0,
				MaximumInterval:    time.Second,
				MaximumAttempts:    int32(in.GetRouteMaxAttempts),
			},
		})
		// An non-nil err indicates we are at the maximum GetRoute attempts. Thus, we return an error message and
		// terminate the workflow. Not all is lost. It can be manually restarted. :)
		if err := workflow.ExecuteActivity(activityCtx, GetRouteActivity, GetRouteIn{Packet: packet}).Get(ctx, &getRouteOut); err != nil {
			return DataRoutingOut{}, fmt.Errorf("Gave up on GetRouteActivity after %d tries", in.GetRouteMaxAttempts)
		}
		// When getRouteOut.Empty is true, it means the service affirmed that it cannot find any more route providers to try.
		// This can happen, for example, if a certain destination has very few route provider. Note that this is not modelled
		// as an error because it is part of the normal business flow.
		if getRouteOut.Empty {
			packet.Status = StatusNoRoute
			return DataRoutingOut{Packet: packet}, nil
		}
		packet.CurrentRouteProvider = getRouteOut.Route

		// In this stanza, the code calls an activity to try to transmit the packet. The activity succeeds only if some remote
		// systems makes a call back to the activity. Drilling down into TransmitActivity will show that it is implemented
		// using asynchronous completion.
		activityCtx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			ScheduleToStartTimeout: time.Minute,             // Timeout between activity being scheduled to starting execution
			StartToCloseTimeout:    TransmitActivityTimeout, // Timeout of the execution
			RetryPolicy:            &temporal.RetryPolicy{BackoffCoefficient: 1.0, MaximumAttempts: 1},
		})
		var transmitOut TransmitOut
		if err := workflow.ExecuteActivity(activityCtx, TransmitActivity, TransmitIn{Packet: packet}).Get(ctx, &transmitOut); err != nil {
			msg := fmt.Sprintf("TransmitActivity failed for route provider: %s", packet.CurrentRouteProvider)
			logger.Error(msg, zap.Error(err))
			// Unlike the we retry GetRouteActivity, we continue the loop so that we pick a new route provider using GetRouteActivity.
			// An alternative to loop-continue is to model the body of the loop as child workflow. We'll save that for another
			// example.
		}

		// If transmitOut says the packet was not delviered, we add the current route provider to the "failed" list so we don't
		// retry it again. Otherwise, we mark the packet as delivered so the loop terminates.
		if !transmitOut.Delivered {
			packet.FailedRouteProviders = append(packet.FailedRouteProviders, packet.CurrentRouteProvider)
			packet.CurrentRouteProvider = ""
		} else {
			packet.Status = StatusDelivered
		}
	}

	// Return the updated packet using the DataRoutingOut container
	return DataRoutingOut{Packet: packet}, nil
}
