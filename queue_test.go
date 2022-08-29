package frame_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/pitabwire/frame"
	"log"
	"testing"
	"time"
)

func TestService_RegisterPublisherNotSet(t *testing.T) {
	ctx := context.Background()

	srv := frame.NewService("Test Srv")

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}

func TestService_RegisterPublisherNotInitialized(t *testing.T) {
	ctx := context.Background()
	opt := frame.RegisterPublisher("test", "mem://topicA")
	srv := frame.NewService("Test Srv", opt)

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}

func TestService_RegisterPublisher(t *testing.T) {
	ctx := context.Background()

	opt := frame.RegisterPublisher("test", "mem://topicA")
	srv := frame.NewService("Test Srv", opt, frame.NoopDriver())

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte(""))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}
}

func TestService_RegisterPublisherMultiple(t *testing.T) {
	ctx := context.Background()

	topicRef := "test-multiple-publisher"
	topicRef2 := "test-multiple-publisher-2"

	opt := frame.RegisterPublisher(topicRef, "mem://topicA")
	opt1 := frame.RegisterPublisher(topicRef2, "mem://topicB")
	srv := frame.NewService("Test Srv", opt, opt1, frame.NoopDriver())

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, topicRef, []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
		return
	}

	err = srv.Publish(ctx, topicRef2, []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
		return
	}

	err = srv.Publish(ctx, "test-multiple-3", []byte("Testament"))
	if err == nil {
		t.Errorf("We should not be able to publish to topic that was not registered")
		return
	}
}

type messageHandler struct {
}

func (m *messageHandler) Handle(ctx context.Context, message []byte) error {
	log.Printf(" A nice message to handle: %v", string(message))
	return nil
}

type handlerWithError struct {
}

func (m *handlerWithError) Handle(ctx context.Context, message []byte) error {
	log.Printf(" A dreadful message to handle: %v", string(message))
	return errors.New("throwing an error for tests")

}

func TestService_RegisterSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	regSubTopic := "test-reg-sub-topic"

	optTopic := frame.RegisterPublisher(regSubTopic, "mem://topicA")
	opt := frame.RegisterSubscriber(regSubTopic, "mem://topicA",
		5, &messageHandler{})

	srv := frame.NewService("Test Srv", optTopic, opt, frame.NoopDriver())
	defer srv.Stop(ctx)

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	for i := range make([]int, 30) {
		err = srv.Publish(ctx, regSubTopic, []byte(fmt.Sprintf(" testing message %d", i)))
		if err != nil {
			t.Errorf("We could not publish to topic that was registered %+v", err)
			return
		}
	}

	err = srv.Publish(ctx, regSubTopic, []byte("throw error"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
		return
	}

}

func TestService_RegisterSubscriberWithError(t *testing.T) {
	ctx := context.Background()

	regSubT := "reg_s_wit-error"
	opt := frame.RegisterSubscriber(regSubT, "mem://topicErrors", 1, &handlerWithError{})
	optTopic := frame.RegisterPublisher(regSubT, "mem://topicErrors")

	srv := frame.NewService("Test Srv", opt, optTopic, frame.NoopDriver())
	defer srv.Stop(ctx)

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, regSubT, []byte(" testing message with error"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
		return
	}
}

func TestService_RegisterSubscriberInvalid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := frame.RegisterSubscriber("test", "memt+://topicA",
		5, &messageHandler{})

	srv := frame.NewService("Test Srv", opt, frame.NoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err == nil {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}

func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	noopDriver := frame.NoopDriver()
	optTopic := frame.RegisterPublisher("test", "mem://topicA")
	opt := frame.RegisterSubscriber("test", "mem://topicA",
		5, &messageHandler{})

	srv := frame.NewService("Test Srv", opt, optTopic, noopDriver)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	if !srv.SubscriptionIsInitiated("test") {
		t.Errorf("Subscription is invalid yet it should be ok")
	}

	cancel()
	time.Sleep(3 * time.Second)
	if srv.SubscriptionIsInitiated("test") {
		t.Errorf("Subscription is valid yet it should not be ok")
	}

}
