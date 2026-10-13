package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

type syncManagerStub struct {
	calls        []string
	desired      *protocol.DesiredState
	syncErr      error
	heartbeatErr error
}

func (s *syncManagerStub) Sync(context.Context) (*protocol.DesiredState, error) {
	s.calls = append(s.calls, "sync")
	return s.desired, s.syncErr
}

func (s *syncManagerStub) Heartbeat(context.Context) error {
	s.calls = append(s.calls, "heartbeat")
	return s.heartbeatErr
}

func TestRunSyncReportsSuccessAfterHeartbeat(t *testing.T) {
	stub := &syncManagerStub{desired: &protocol.DesiredState{UnsignedDesiredState: protocol.UnsignedDesiredState{
		Revision: 7, DesiredState: "active",
	}}}
	var output bytes.Buffer

	if err := runSync(stub, &output); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if strings.Join(stub.calls, ",") != "sync,heartbeat" {
		t.Fatalf("calls = %v, want sync then heartbeat", stub.calls)
	}
	if !strings.Contains(output.String(), "revision=7 state=active") {
		t.Fatalf("success output = %q", output.String())
	}
}

func TestRunSyncDoesNotHeartbeatWhenDesiredStateIsUnavailable(t *testing.T) {
	syncErr := errors.New("sync failed")
	stub := &syncManagerStub{syncErr: syncErr}
	var output bytes.Buffer

	err := runSync(stub, &output)
	if !errors.Is(err, syncErr) {
		t.Fatalf("error = %v, want sync error", err)
	}
	if strings.Join(stub.calls, ",") != "sync" {
		t.Fatalf("calls = %v, want sync only", stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("failure must not print success, got %q", output.String())
	}
}

func TestRunSyncHeartbeatsAfterConvergenceFailureAndReturnsError(t *testing.T) {
	syncErr := errors.New("local convergence failed")
	stub := &syncManagerStub{
		desired: &protocol.DesiredState{UnsignedDesiredState: protocol.UnsignedDesiredState{
			Revision: 9, DesiredState: "active",
		}},
		syncErr: syncErr,
	}
	var output bytes.Buffer

	err := runSync(stub, &output)

	if !errors.Is(err, syncErr) {
		t.Fatalf("error = %v, want convergence error so CLI exits non-zero", err)
	}
	if strings.Join(stub.calls, ",") != "sync,heartbeat" {
		t.Fatalf("calls = %v, want failed convergence state heartbeated before returning", stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("failed convergence must not print success, got %q", output.String())
	}
}

func TestRunSyncDoesNotReportSuccessAfterHeartbeatFailure(t *testing.T) {
	heartbeatErr := errors.New("heartbeat failed")
	stub := &syncManagerStub{
		desired: &protocol.DesiredState{UnsignedDesiredState: protocol.UnsignedDesiredState{
			Revision: 8, DesiredState: "active",
		}},
		heartbeatErr: heartbeatErr,
	}
	var output bytes.Buffer

	err := runSync(stub, &output)
	if !errors.Is(err, heartbeatErr) || !strings.Contains(err.Error(), "立即心跳上报失败") {
		t.Fatalf("error = %v, want explicit heartbeat failure", err)
	}
	if strings.Join(stub.calls, ",") != "sync,heartbeat" {
		t.Fatalf("calls = %v, want sync then heartbeat", stub.calls)
	}
	if output.Len() != 0 {
		t.Fatalf("heartbeat failure must not print success, got %q", output.String())
	}
}
