package main

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"testing"
)

type UnitTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *UnitTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *UnitTestSuite) AfterTest(string, string) {
	s.env.AssertExpectations(s.T())
}

func (s *UnitTestSuite) Test_DataRoutingWorkflow_ok() {
	in := DataRoutingIn{
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

	s.env.OnActivity(GetRouteActivity, mock.Anything, mock.Anything).Return(
		GetRouteOut{
			Empty: false,
			Route: "MockRoute",
		}, nil)
	s.env.OnActivity(TransmitActivity, mock.Anything, mock.Anything).Return(
		TransmitOut{
			Delivered: true,
		}, nil)

	s.env.ExecuteWorkflow(DataRoutingWorkflow, in)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *UnitTestSuite) Test_DataRoutingWorkflow_MultipleTransmit_ok() {
	// The second call to GetRouteActivity will return a goodRoute
	getRouteOutSeq := []GetRouteOut {
		{false, "badRoute"},
		{false, "goodRoute"},
	}
	getRouteActivityMock := func(context.Context, GetRouteIn) (GetRouteOut, error) {
		r := getRouteOutSeq[0]
		getRouteOutSeq = getRouteOutSeq[1:]
		return r, nil
	}
	transmitActivityMock := func(_ context.Context, in TransmitIn) (TransmitOut, error) {
		if in.Packet.CurrentRouteProvider == "goodRoute" {
			return TransmitOut{	Delivered: true	}, nil
		}
		return TransmitOut{Delivered: false}, fmt.Errorf("activity timed out")
	}

	in := DataRoutingIn{
		GetRouteMaxAttempts:       1,
		TransmitPacketMaxAttempts: 2,
	}

	s.env.OnActivity(GetRouteActivity, mock.Anything, mock.Anything).Return(getRouteActivityMock)
	s.env.OnActivity(TransmitActivity, mock.Anything, mock.Anything).Return(transmitActivityMock)

	s.env.ExecuteWorkflow(DataRoutingWorkflow, in)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	var dataRoutingOut DataRoutingOut
	s.NoError(s.env.GetWorkflowResult(&dataRoutingOut))
	s.Equal([]string{"badRoute"}, dataRoutingOut.Packet.FailedRouteProviders)
	s.Equal("goodRoute", dataRoutingOut.Packet.CurrentRouteProvider)
	s.Equal(StatusDelivered, dataRoutingOut.Packet.Status)
}

func (s *UnitTestSuite) Test_DataRoutingWorkflow_MultipleTransmit_fail() {
	// The second call to GetRouteActivity will return a goodRoute
	getRouteOutSeq := []GetRouteOut {
		{false, "badRoute"},
		{true, ""},
	}
	getRouteActivityMock := func(context.Context, GetRouteIn) (GetRouteOut, error) {
		r := getRouteOutSeq[0]
		getRouteOutSeq = getRouteOutSeq[1:]
		return r, nil
	}
	transmitActivityMock := func(_ context.Context, in TransmitIn) (TransmitOut, error) {
		return TransmitOut{Delivered: false}, fmt.Errorf("activity timed out")
	}

	in := DataRoutingIn{
		GetRouteMaxAttempts:       1,
		TransmitPacketMaxAttempts: 2,
	}

	s.env.OnActivity(GetRouteActivity, mock.Anything, mock.Anything).Return(getRouteActivityMock)
	s.env.OnActivity(TransmitActivity, mock.Anything, mock.Anything).Return(transmitActivityMock)

	s.env.ExecuteWorkflow(DataRoutingWorkflow, in)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	var dataRoutingOut DataRoutingOut
	s.NoError(s.env.GetWorkflowResult(&dataRoutingOut))
	s.Equal([]string{"badRoute"}, dataRoutingOut.Packet.FailedRouteProviders)
	s.Equal("", dataRoutingOut.Packet.CurrentRouteProvider)
	s.Equal(StatusNoRoute, dataRoutingOut.Packet.Status)
}


func TestUnitTestSuite(t *testing.T) {
	suite.Run(t, new(UnitTestSuite))
}
