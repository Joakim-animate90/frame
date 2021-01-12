package frame

import (
	"context"
	"errors"
	"fmt"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	"log"
)

type Queue struct {
	publishQueueMap      map[string]*publisher
	subscriptionQueueMap map[string]*subscriber
}

type publisher struct {
	url      string
	pubTopic *pubsub.Topic
	isInit   bool
}

type subscriber struct {
	url            string
	concurrency    int
	subscFunction  func(message []byte, metadata map[string]string) error
	subscription   *pubsub.Subscription
	isInit         bool
}

func RegisterPublisher(reference string, queueUrl string) Option {
	return func(s *Service) {

		if s.queue.publishQueueMap == nil {
			s.queue.publishQueueMap = make(map[string]*publisher)
		}

		s.queue.publishQueueMap[reference] = &publisher{
			url:    queueUrl,
			isInit: false,
		}

	}
}

func RegisterSubscriber(reference string, queueUrl string, concurrency int,
	subscFunc func(message []byte, metadata map[string]string) error) Option {
	return func(s *Service) {
		if s.queue.subscriptionQueueMap == nil {
			s.queue.subscriptionQueueMap = make(map[string]*subscriber)
		}

		s.queue.subscriptionQueueMap[reference] = &subscriber{
			url:            queueUrl,
			concurrency:    concurrency,
			subscFunction:  subscFunc,
		}

	}
}


func (s Service) Publish(ctx context.Context, reference string, message []byte, metadata map[string]string) error {

	publisher := s.queue.publishQueueMap[reference]
	if publisher == nil {
		return errors.New(fmt.Sprintf("Publish -- you need to register a queue : [%v] first before publishing ", reference))
	}

	if !publisher.isInit {
		return errors.New(fmt.Sprintf("Publish -- can't publish on uninitialized queue %v ", reference))
	}

	return publisher.pubTopic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})
}


func (s Service) initPubsub(ctx context.Context) error {

	for ref, publisher := range s.queue.publishQueueMap {

		topic, err := pubsub.OpenTopic(ctx, publisher.url)
		if err != nil {
			return err
		}

		s.AddCleanupMethod(func() {
			err := topic.Shutdown(ctx)
			if err != nil {
				log.Printf("initPubsub -- publish topic %s could not be closed : %v", ref, err)
			}
		})

		publisher.pubTopic = topic
		publisher.isInit = true
	}

	for ref, subscriber := range s.queue.subscriptionQueueMap {
		subs, err := pubsub.OpenSubscription(ctx, subscriber.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %v", err)
		}

		s.AddCleanupMethod(func() {
			err := subs.Shutdown(ctx)
			if err != nil {
				log.Printf("Subscribe -- subscription %s could not be stopped well : %v", ref, err)
			}
		})

		subscriber.subscription = subs
		subscriber.isInit = true

	}

	if len(s.queue.subscriptionQueueMap) > 0 {
		s.subscribe(ctx)
	}

	return nil

}

func (s Service) subscribe(ctx context.Context) {

	for _, subsc := range s.queue.subscriptionQueueMap {

		go func(localSub *subscriber) {


			log.Printf(" subscribe -- Initiating queue : %v", localSub.url)
			sem := make(chan struct{}, localSub.concurrency)
		recvLoop:
			for {
				msg, err := localSub.subscription.Receive(ctx)
				if err != nil {
					// Errors from Receive indicate that Receive will no longer succeed.
					log.Printf(" subscribe -- Could not pull message because : %v", err)
					localSub.isInit = false
					return
				}

				// Wait if there are too many active handle goroutines and acquire the
				// semaphore. If the context is canceled, stop waiting and start shutting
				// down.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					break recvLoop
				}

				go func() {
					defer func() { <-sem }() // Release the semaphore.

					err := localSub.subscFunction(msg.Body, msg.Metadata)
					if err != nil {
						log.Printf(" subscribe -- Unable to process message %v : %v",
							localSub.url, err)
						msg.Nack()
						return
					}

					msg.Ack()

				}()
			}

			// We're no longer receiving messages. Wait to finish handling any
			// unacknowledged messages by totally acquiring the semaphore.
			for n := 0; n < localSub.concurrency; n++ {
				sem <- struct{}{}
			}

			localSub.isInit = false

		}(subsc)

	}

}