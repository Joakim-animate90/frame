package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"strings"
)

type queue struct {
	publishQueueMap      map[string]*publisher
	subscriptionQueueMap map[string]*subscriber
}

func (q queue) getPublisherByReference(reference string) (*publisher, error) {
	p := q.publishQueueMap[reference]
	if p == nil {
		return nil, fmt.Errorf("reference does not exist")
	}
	return p, nil
}

func newQueue() *queue {
	q := &queue{
		publishQueueMap:      make(map[string]*publisher),
		subscriptionQueueMap: make(map[string]*subscriber),
	}

	return q
}

type publisher struct {
	reference string
	url       string
	topic     *pubsub.Topic
}

type SubscribeWorker interface {
	Handle(ctx context.Context, message []byte) error
}

type subscriber struct {
	reference    string
	url          string
	concurrency  int
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       bool
}

// RegisterPublisher Option to register publishing path referenced within the system
func RegisterPublisher(reference string, queueURL string) Option {
	return func(s *Service) {
		s.queue.publishQueueMap[reference] = &publisher{
			reference: reference,
			url:       queueURL,
		}
	}
}

// RegisterSubscriber Option to register a new subscription handler
func RegisterSubscriber(reference string, queueURL string, concurrency int,
	handler SubscribeWorker) Option {
	return func(s *Service) {
		s.queue.subscriptionQueueMap[reference] = &subscriber{
			reference:   reference,
			url:         queueURL,
			concurrency: concurrency,
			handler:     handler,
		}
	}
}

func (s *Service) SubscriptionIsInitiated(path string) bool {
	return s.queue.subscriptionQueueMap[path].isInit
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference
func (s *Service) Publish(ctx context.Context, reference string, payload interface{}) error {
	var metadata map[string]string

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		metadata = authClaim.AsMetadata()
	} else {
		metadata = make(map[string]string)
	}

	pub, err := s.queue.getPublisherByReference(reference)
	if err != nil {
		if err.Error() != "reference does not exist" {
			return err
		}

		if !strings.Contains(reference, "://") {
			return err
		}

		pub = &publisher{
			reference: reference,
			url:       reference,
		}
		err = s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
		s.queue.publishQueueMap[reference] = pub
	}

	var message []byte
	msg, ok := payload.([]byte)
	if !ok {
		msg, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		message = msg
	} else {
		message = msg
	}

	return pub.topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})

}

func (s *Service) initPublisher(ctx context.Context, pub *publisher) error {
	topic, err := pubsub.OpenTopic(ctx, pub.url)
	if err != nil {
		return err
	}
	s.AddCleanupMethod(func(ctx context.Context) {
		err = topic.Shutdown(ctx)
		if err != nil {
			s.L().WithError(err).WithField("reference", pub.reference).Warn("topic could not be closed")
		}
	})
	pub.topic = topic
	return nil
}
func (s *Service) initSubscriber(ctx context.Context, sub *subscriber) error {
	if !strings.HasPrefix(sub.url, "http") {
		subsc, err := pubsub.OpenSubscription(ctx, sub.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %+v", err)
		}

		s.AddCleanupMethod(func(ctx context.Context) {
			err = subsc.Shutdown(ctx)
			if err != nil {
				s.L().WithError(err).WithField("reference", sub.reference).Warn("subscription could not be stopped")
			}
		})

		sub.subscription = subsc
	}
	sub.isInit = true
	return nil
}

func (s *Service) initPubsub(ctx context.Context) error {
	// Whenever the registry is not empty the events queue is automatically initiated
	if s.eventRegistry != nil && len(s.eventRegistry) > 0 {
		eventsQueueHandler := eventQueueHandler{
			service: s,
		}
		eventsQueueURL := GetEnv(envEventsQueueUrl, fmt.Sprintf("mem://%s", eventsQueueName))
		eventsQueue := RegisterSubscriber(eventsQueueName, eventsQueueURL, 10, &eventsQueueHandler)
		eventsQueue(s)
		eventsQueueP := RegisterPublisher(eventsQueueName, eventsQueueURL)
		eventsQueueP(s)
	}

	if s.queue == nil {
		return nil
	}

	for _, pub := range s.queue.publishQueueMap {
		err := s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
	}

	for _, sub := range s.queue.subscriptionQueueMap {
		err := s.initSubscriber(ctx, sub)
		if err != nil {
			return err
		}
	}

	if len(s.queue.subscriptionQueueMap) > 0 {
		s.subscribe(ctx)
	}

	return nil
}

func (s *Service) subscribe(ctx context.Context) {
	for _, subsc := range s.queue.subscriptionQueueMap {
		logger := s.L().WithField("subscriber", subsc.reference).WithField("url", subsc.url)

		if strings.HasPrefix(subsc.url, "http") {
			continue
		}

		go func(logger *logrus.Entry, localSub *subscriber) {
			sem := make(chan struct{}, localSub.concurrency)
		recvLoop:
			for {
				// Wait if there are too many active handle goroutines and acquire the
				// semaphore. If the context is canceled, stop waiting and start shutting
				// down.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					break recvLoop
				}

				msg, err := localSub.subscription.Receive(ctx)
				if err != nil {
					logger.WithError(err).Error(" could not pull message")
					localSub.isInit = false
					continue
				}

				go func(logger *logrus.Entry) {
					defer func() { <-sem }() // Release the semaphore.

					authClaim := ClaimsFromMap(msg.Metadata)

					if nil != authClaim {
						ctx = authClaim.ClaimsToContext(ctx)
					}

					ctx = ToContext(ctx, s)

					err := localSub.handler.Handle(ctx, msg.Body)
					if err != nil {
						logger.WithError(err).Error("unable to process message")
						msg.Nack()
						return
					}

					msg.Ack()
				}(logger)
			}

			// We're no longer receiving messages. Wait to finish handling any
			// unacknowledged messages by totally acquiring the semaphore.
			for n := 0; n < localSub.concurrency; n++ {
				sem <- struct{}{}
			}
			localSub.isInit = false
		}(logger, subsc)
	}
}
