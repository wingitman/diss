package project

import "testing"

func TestBurnWriterParsesProgress(t *testing.T) {
	var events []BurnEvent
	writer := &burnWriter{emit: func(event BurnEvent) { events = append(events, event) }, phase: "Writing", start: 30, end: 95}
	if _, err := writer.Write([]byte("Track 01: 50 of 100 MB\r")); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Progress != 62 {
		t.Fatalf("unexpected progress events: %+v", events)
	}
}

func TestBurnWriterKeepsPartialLines(t *testing.T) {
	var events []BurnEvent
	writer := &burnWriter{emit: func(event BurnEvent) { events = append(events, event) }, phase: "Writing", start: 0, end: 100}
	_, _ = writer.Write([]byte("first"))
	_, _ = writer.Write([]byte(" line\n"))
	if len(events) != 1 || events[0].Line != "first line" {
		t.Fatalf("unexpected lines: %+v", events)
	}
}
