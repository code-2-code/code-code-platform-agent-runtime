package nats

import "testing"

func TestNewPublisherRejectsEmptyURL(t *testing.T) {
	_, err := NewPublisher(PublisherConfig{SubjectPrefix: "platform.timeline"})
	if err == nil {
		t.Fatal("NewPublisher() expected error, got nil")
	}
}
