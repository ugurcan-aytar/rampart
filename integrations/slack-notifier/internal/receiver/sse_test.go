package receiver_test

import (
	"strings"
	"testing"

	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/receiver"
)

func TestReadStream_SingleFrame(t *testing.T) {
	input := "id: INC1\nevent: incident.opened\ndata: {\"incidentId\":\"INC1\",\"type\":\"incident.opened\"}\n\n"
	var got []receiver.Frame
	if err := receiver.ReadStream(strings.NewReader(input), func(f receiver.Frame) {
		got = append(got, f)
	}); err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 frame, got %d", len(got))
	}
	if got[0].ID != "INC1" || got[0].EventType != "incident.opened" {
		t.Errorf("bad frame: %+v", got[0])
	}
	if !strings.Contains(got[0].Data, "INC1") {
		t.Errorf("data missing: %q", got[0].Data)
	}
}

func TestReadStream_MultipleFrames(t *testing.T) {
	input := "" +
		"id: INC1\nevent: incident.opened\ndata: {\"type\":\"incident.opened\"}\n\n" +
		"id: INC1\nevent: incident.transitioned\ndata: {\"type\":\"incident.transitioned\"}\n\n"
	var got []receiver.Frame
	if err := receiver.ReadStream(strings.NewReader(input), func(f receiver.Frame) {
		got = append(got, f)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 frames, got %d", len(got))
	}
	if got[0].EventType != "incident.opened" || got[1].EventType != "incident.transitioned" {
		t.Errorf("wrong frames: %+v", got)
	}
}

func TestReadStream_SkipsHeartbeats(t *testing.T) {
	input := ": keep-alive\n\n: keep-alive\n\nid: INC1\nevent: incident.opened\ndata: {}\n\n"
	var got []receiver.Frame
	if err := receiver.ReadStream(strings.NewReader(input), func(f receiver.Frame) {
		got = append(got, f)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("heartbeats must not produce frames; got %d", len(got))
	}
}
